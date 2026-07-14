package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Adapter struct{}

// NetworkSpec holds all parameters for creating a Podman network.
type NetworkSpec struct {
	Name          string
	Subnet        string
	HostGateway   string
	DNS           string
	Driver        string            // empty = Podman default (bridge)
	DriverOptions map[string]string // passed as -o key=val; keys sorted
	IPAMDriver    string            // optional custom IPAM driver
}

type ImageBuildRequest struct {
	Tag       string
	Context   string
	File      string
	NoCache   bool
	Platforms []string
}

type ImagePullRequest struct {
	Reference string
}

type ImagePushRequest struct {
	Reference string
}

type ImageTagRequest struct {
	Source string
	Target string
}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) EnsureNetwork(ctx context.Context, spec NetworkSpec) error {
	inspect := exec.CommandContext(ctx, "podman", "network", "exists", spec.Name)
	if err := inspect.Run(); err == nil {
		return nil
	}
	args := buildNetworkArgs(spec)
	cmd := exec.CommandContext(ctx, "podman", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	// Two conflict patterns:
	//  "already used" / "already exists" – same-name race or subnet taken by another network
	//  "already used on the host" – kernel bridge still present after a prior rm
	isConflict := strings.Contains(msg, "already used") ||
		strings.Contains(msg, "already exists")
	if isConflict {
		if recheck := exec.CommandContext(ctx, "podman", "network", "exists", spec.Name); recheck.Run() == nil {
			return nil
		}
		// Subnet is owned by a different network — find and force-remove it
		// (--force disconnects any attached containers), then retry.
		if spec.Subnet != "" {
			stale, bridge, _ := findNetworkBySubnet(ctx, spec.Subnet)
			if stale != "" {
				exec.CommandContext(ctx, "podman", "network", "rm", "--force", stale).CombinedOutput() //nolint:errcheck
			}
			// Delete the kernel bridge — it may linger even after the Podman
			// record is gone because running containers still hold veth pairs.
			if bridge != "" {
				deleteKernelLink(ctx, bridge)
			} else {
				// Orphaned bridge with no Podman record: find by subnet.
				deleteOrphanedBridge(ctx, spec.Subnet)
			}
			// Retry with back-off.
			var retryErr error
			for attempt := 0; attempt < 5; attempt++ {
				if attempt > 0 {
					time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				}
				retry := exec.CommandContext(ctx, "podman", args...)
				var retryOut []byte
				retryOut, retryErr = retry.CombinedOutput()
				if retryErr == nil {
					return nil
				}
				retryMsg := strings.TrimSpace(string(retryOut))
				if !strings.Contains(retryMsg, "already used") && !strings.Contains(retryMsg, "already exists") {
					return fmt.Errorf("create podman network %s: %w (%s)", spec.Name, retryErr, retryMsg)
				}
			}
			return fmt.Errorf("create podman network %s: subnet %s still in use after retries", spec.Name, spec.Subnet)
		}
		return fmt.Errorf("create podman network %s: subnet %s is already in use by another network", spec.Name, spec.Subnet)
	}
	return fmt.Errorf("create podman network %s: %w (%s)", spec.Name, err, msg)
}

// buildNetworkArgs constructs the podman network create argument list from a NetworkSpec.
func buildNetworkArgs(spec NetworkSpec) []string {
	args := []string{"network", "create"}
	if spec.Driver != "" {
		args = append(args, "--driver", spec.Driver)
	}
	// Driver options: sort keys for deterministic args.
	if len(spec.DriverOptions) > 0 {
		keys := make([]string, 0, len(spec.DriverOptions))
		for k := range spec.DriverOptions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "-o", k+"="+spec.DriverOptions[k])
		}
	}
	if spec.IPAMDriver != "" {
		args = append(args, "--ipam-driver", spec.IPAMDriver)
	}
	// Subnet, gateway, and DNS are Podman-IPAM concepts. When ipam-driver=none
	// the container manages its own IPs and Podman rejects these flags.
	if strings.TrimSpace(spec.Subnet) != "" && spec.IPAMDriver != "none" {
		args = append(args, "--subnet", spec.Subnet)
		if strings.TrimSpace(spec.HostGateway) != "" {
			args = append(args, "--gateway", spec.HostGateway)
		}
		if strings.TrimSpace(spec.DNS) != "" {
			args = append(args, "--dns", spec.DNS)
		}
	}
	args = append(args, spec.Name)
	return args
}

// RemoveNetwork removes a Podman network by name. Returns an error if the
// network cannot be removed (e.g., containers still connected). Callers should
// treat errors as warnings and continue teardown.
func (a *Adapter) RemoveNetwork(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "podman", "network", "rm", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove podman network %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findNetworkBySubnet returns the name of the Podman network whose subnet
// interface name for the network whose subnet matches cidr.
// Returns ("", "", nil) if none is found.
func findNetworkBySubnet(ctx context.Context, cidr string) (name, bridge string, err error) {
	out, runErr := exec.CommandContext(ctx, "podman", "network", "ls", "--format", "json").CombinedOutput()
	if runErr != nil {
		return "", "", fmt.Errorf("podman network ls: %w", runErr)
	}
	var nets []struct {
		Name             string `json:"name"`
		NetworkInterface string `json:"network_interface"`
		Subnets          []struct {
			Subnet string `json:"subnet"`
		} `json:"subnets"`
	}
	if jsonErr := json.Unmarshal(out, &nets); jsonErr != nil {
		return "", "", fmt.Errorf("parse podman network ls: %w", jsonErr)
	}
	for _, n := range nets {
		for _, s := range n.Subnets {
			if s.Subnet == cidr {
				return n.Name, n.NetworkInterface, nil
			}
		}
	}
	return "", "", nil
}

// deleteKernelLink forcibly removes a kernel network interface by name.
func deleteKernelLink(ctx context.Context, iface string) {
	if runtime.GOOS == "darwin" {
		exec.CommandContext(ctx, "podman", "machine", "ssh", "--", "ip", "link", "delete", iface).CombinedOutput() //nolint:errcheck
	} else {
		exec.CommandContext(ctx, "ip", "link", "delete", iface).CombinedOutput() //nolint:errcheck
	}
}

// deleteOrphanedBridge removes a kernel bridge that holds cidr but has no
// associated Podman network record. On macOS this runs via `podman machine ssh`;
// on Linux it runs ip(8) directly.
func deleteOrphanedBridge(ctx context.Context, cidr string) error {
	ipRun := func(args ...string) ([]byte, error) {
		if runtime.GOOS == "darwin" {
			return exec.CommandContext(ctx, "podman", append([]string{"machine", "ssh", "--"}, args...)...).CombinedOutput()
		}
		return exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	}
	routeOut, err := ipRun("ip", "-o", "route", "show", cidr)
	if err != nil {
		return fmt.Errorf("ip route show %s: %w", cidr, err)
	}
	// Output: "<cidr> dev <iface> ..."
	iface := ""
	fields := strings.Fields(strings.TrimSpace(string(routeOut)))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
			break
		}
	}
	if iface == "" {
		return fmt.Errorf("no interface found for subnet %s", cidr)
	}
	_, err = ipRun("ip", "link", "delete", iface)
	return err
}

func (a *Adapter) EnsureVolume(ctx context.Context, name string) error {
	inspect := exec.CommandContext(ctx, "podman", "volume", "exists", name)
	if err := inspect.Run(); err == nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "podman", "volume", "create", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create podman volume %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) EnsureContainer(ctx context.Context, name, image string, args ...string) error {
	inspect := exec.CommandContext(ctx, "podman", "container", "exists", name)
	if err := inspect.Run(); err == nil {
		return nil
	}
	cmdArgs := append([]string{"run", "-d", "--name", name}, args...)
	cmdArgs = append(cmdArgs, image)
	cmd := exec.CommandContext(ctx, "podman", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create podman container %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) RemoveContainer(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "podman", "rm", "-f", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove podman container %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) Ping(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "podman", "info", "--format", "json")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("podman info failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) ImageExists(ctx context.Context, reference string) (bool, error) {
	cmd := exec.CommandContext(ctx, "podman", "image", "exists", reference)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("check podman image %s: %w", reference, err)
	}
	return true, nil
}

func (a *Adapter) BuildImage(ctx context.Context, req ImageBuildRequest) error {
	// When building a multi-arch manifest list, remove any existing image or
	// manifest with the same tag first. podman build --manifest fails if the
	// name already exists as a regular (single-arch) image.
	if len(req.Platforms) > 0 {
		exec.CommandContext(ctx, "podman", "manifest", "rm", req.Tag).Run() //nolint:errcheck
		exec.CommandContext(ctx, "podman", "rmi", "--force", req.Tag).Run() //nolint:errcheck
	}
	args, err := buildImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build podman image %s: %w", req.Tag, err)
	}
	return nil
}

func (a *Adapter) PullImage(ctx context.Context, req ImagePullRequest) error {
	args, err := pullImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pull podman image %s: %w", req.Reference, err)
	}
	return nil
}

func (a *Adapter) PushImage(ctx context.Context, req ImagePushRequest) error {
	args, err := pushImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "podman", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("push podman image %s: %w (%s)", req.Reference, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) TagImage(ctx context.Context, req ImageTagRequest) error {
	args, err := tagImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "podman", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tag podman image %s -> %s: %w (%s)", req.Source, req.Target, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func buildImageArgs(req ImageBuildRequest) ([]string, error) {
	if req.Tag == "" {
		return nil, fmt.Errorf("build image tag is required")
	}
	if req.Context == "" {
		return nil, fmt.Errorf("build context is required")
	}
	var args []string
	if len(req.Platforms) > 0 {
		args = []string{"build", "--platform", strings.Join(req.Platforms, ","), "--manifest", req.Tag}
	} else {
		args = []string{"build", "-t", req.Tag}
	}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	if req.File != "" {
		args = append(args, "-f", req.File)
	}
	args = append(args, req.Context)
	return args, nil
}

func pullImageArgs(req ImagePullRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("pull reference is required")
	}
	return []string{"pull", req.Reference}, nil
}

func pushImageArgs(req ImagePushRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("push reference is required")
	}
	return []string{"push", req.Reference}, nil
}

func tagImageArgs(req ImageTagRequest) ([]string, error) {
	if req.Source == "" {
		return nil, fmt.Errorf("tag source is required")
	}
	if req.Target == "" {
		return nil, fmt.Errorf("tag target is required")
	}
	return []string{"tag", req.Source, req.Target}, nil
}

// lastUsableIP returns the last usable host address in a CIDR block. For
// 192.168.10.0/24 this is 192.168.10.254. Podman is told to assign this IP to
// its host-side bridge so the network gateway (.1) stays free for the gateway
// container's brlan0 bridge interface.
func lastUsableIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		ip = ipNet.IP.To16()
	}
	// Compute broadcast: network | ^mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^ipNet.Mask[i]
	}
	// Last usable = broadcast - 1
	last := make(net.IP, len(broadcast))
	copy(last, broadcast)
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] > 0 {
			last[i]--
			break
		}
	}
	return last.String(), nil
}

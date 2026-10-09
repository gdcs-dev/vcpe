package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
)

// Adapter implements image.Backend directly for Podman image operations and
// owns the Podman-specific networking operations in this package.
type Adapter struct {
	run commandOutputRunner
}

type commandOutputRunner func(context.Context, string, ...string) ([]byte, error)

var _ image.Backend = (*Adapter)(nil)

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

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if a != nil && a.run != nil {
		return a.run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// ContainerPID returns the VM-side PID for a running Podman container.
func (a *Adapter) ContainerPID(ctx context.Context, name string) (int, error) {
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("container name is required")
	}
	out, err := a.output(ctx, "podman", "inspect", "--type", "container", "--format", "{{json .State}}", name)
	if err != nil {
		return 0, fmt.Errorf("inspect podman container %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	var state struct {
		Running bool `json:"Running"`
		PID     int  `json:"Pid"`
	}
	if err := json.Unmarshal(out, &state); err != nil {
		return 0, fmt.Errorf("decode podman container %s state: %w", name, err)
	}
	if !state.Running {
		return 0, fmt.Errorf("podman container %s is not running", name)
	}
	if state.PID <= 0 {
		return 0, fmt.Errorf("podman container %s has invalid VM-side PID %d", name, state.PID)
	}
	return state.PID, nil
}

// ContainerFile reads a bounded, non-secret runtime contract file from a
// running container. Callers own the path and content contract.
func (a *Adapter) ContainerFile(ctx context.Context, name, path string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("container name is required")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("container file path is required")
	}
	out, err := a.output(ctx, "podman", "exec", name, "cat", path)
	if err != nil {
		return nil, fmt.Errorf("read runtime contract from podman container %s: %w", name, err)
	}
	return out, nil
}

func (a *Adapter) ContainerVAPStatus(ctx context.Context, name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("container name is required")
	}
	out, err := a.output(ctx, "podman", "exec", name, "gateway-health-probe", "vaps")
	if err != nil {
		return out, fmt.Errorf("inspect wireless VAPs in podman container %s: %w", name, err)
	}
	return out, nil
}

func (a *Adapter) ContainerMeshStatus(ctx context.Context, name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("container name is required")
	}
	out, err := a.output(ctx, "podman", "exec", name, "gateway-health-probe", "mesh")
	if err != nil {
		return out, fmt.Errorf("inspect mesh peer in podman container %s: %w", name, err)
	}
	return out, nil
}

func (a *Adapter) ProbeLAN(ctx context.Context, container, device, address string) error {
	if container == "" || device == "" || address == "" {
		return fmt.Errorf("LAN probe requires a container, device, and address")
	}
	_, err := a.output(ctx, "podman", "exec", container, "ping", "-n", "-c", "1", "-W", "1", "-I", device, address)
	if err != nil {
		return fmt.Errorf("LAN probe from %s to %s failed: %w", container, address, err)
	}
	return nil
}

type StationObservation struct {
	BSSID         string
	Authenticated bool
	IPv4          netip.Prefix
}

func (a *Adapter) ObserveStation(ctx context.Context, container, device string) (StationObservation, error) {
	if container == "" || device == "" {
		return StationObservation{}, fmt.Errorf("station observation requires a container and device")
	}
	read := func(args ...string) ([]byte, error) {
		out, err := a.output(ctx, "podman", append([]string{"exec", container}, args...)...)
		if err != nil {
			return nil, fmt.Errorf("observe station %s: %w", container, err)
		}
		return out, nil
	}
	status, err := read("wpa_cli", "-i", device, "status")
	if err != nil {
		return StationObservation{}, err
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	link, err := read("iw", "dev", device, "link")
	if err != nil {
		return StationObservation{}, err
	}
	var linkBSSID string
	for _, line := range strings.Split(string(link), "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "Connected to "); ok {
			if parts := strings.Fields(after); len(parts) > 0 {
				linkBSSID = parts[0]
			}
			break
		}
	}
	addresses, err := read("ip", "-j", "-4", "addr", "show", "dev", device)
	if err != nil {
		return StationObservation{}, err
	}
	var interfaces []struct {
		AddressInfo []struct {
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(addresses, &interfaces); err != nil {
		return StationObservation{}, fmt.Errorf("decode station IPv4 address: %w", err)
	}
	if len(interfaces) != 1 || len(interfaces[0].AddressInfo) != 1 {
		return StationObservation{}, fmt.Errorf("station %s must have exactly one IPv4 address", container)
	}
	address := interfaces[0].AddressInfo[0]
	ipv4, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", address.Local, address.PrefixLen))
	if err != nil || !ipv4.Addr().Is4() {
		return StationObservation{}, fmt.Errorf("station %s has invalid IPv4 address", container)
	}
	observed := StationObservation{IPv4: ipv4}
	if fields["wpa_state"] != "COMPLETED" {
		return observed, nil
	}
	bssid, parseErr := net.ParseMAC(fields["bssid"])
	linkMAC, linkErr := net.ParseMAC(linkBSSID)
	if parseErr != nil || linkErr != nil || !strings.EqualFold(bssid.String(), linkMAC.String()) {
		return observed, nil
	}
	observed.BSSID = bssid.String()
	observed.Authenticated = true
	return observed, nil
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

func (a *Adapter) VerifyMeshGatewayImage(ctx context.Context, reference string) error {
	out, err := a.output(ctx, "podman", "image", "inspect", reference)
	if err != nil {
		return fmt.Errorf("inspect mesh Gateway image %q: %w", reference, err)
	}
	var images []struct {
		RepoDigests []string          `json:"RepoDigests"`
		RepoTags    []string          `json:"RepoTags"`
		Labels      map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(out, &images); err != nil || len(images) != 1 {
		return fmt.Errorf("mesh Gateway image %q has invalid inspection metadata", reference)
	}
	identities := images[0].RepoDigests
	if reference == "ghcr.io/gdcs-dev/gateway:dev" {
		identities = images[0].RepoTags
	}
	for _, identity := range identities {
		if identity == reference && images[0].Labels["org.gdcs-dev.vcpe.gateway.mesh-sae"] == "1" {
			return nil
		}
	}
	return fmt.Errorf("mesh Gateway image %q is not verified for mesh and SAE", reference)
}

func (a *Adapter) BuildImage(ctx context.Context, req image.BuildRequest) error {
	if !req.ArtifactsPrepared {
		if err := image.PrepareBuild(ctx, req); err != nil {
			return err
		}
	}

	// When building a multi-arch manifest list, remove any existing image or
	// manifest with the same tag first. podman build --manifest fails if the
	// name already exists as a regular (single-arch) image.
	if len(req.Platforms) > 1 && len(req.Tags) > 0 {
		exec.CommandContext(ctx, "podman", "manifest", "rm", req.Tags[0]).Run() //nolint:errcheck
		exec.CommandContext(ctx, "podman", "rmi", "--force", req.Tags[0]).Run() //nolint:errcheck
	}

	// Generated binaries were prepared in platform-qualified context paths, so
	// Podman can safely snapshot all requested platforms in one invocation.
	return a.runBuildImage(ctx, req)
}

func (a *Adapter) runBuildImage(ctx context.Context, req image.BuildRequest) error {
	args, err := buildImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		primary := ""
		if len(req.Tags) > 0 {
			primary = req.Tags[0]
		}
		return fmt.Errorf("build podman image %s: %w", primary, err)
	}
	return nil
}

func (a *Adapter) PullImage(ctx context.Context, req image.PullRequest) error {
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

func (a *Adapter) PushImage(ctx context.Context, req image.PushRequest) error {
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

func (a *Adapter) TagImage(ctx context.Context, req image.TagRequest) error {
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

func buildImageArgs(req image.BuildRequest) ([]string, error) {
	if len(req.Tags) == 0 {
		return nil, fmt.Errorf("build image tags are required")
	}
	if req.Context == "" {
		return nil, fmt.Errorf("build context is required")
	}
	var args []string
	if len(req.Platforms) > 1 {
		// Multi-arch: podman build only accepts a single --manifest name.
		args = []string{"build", "--platform", strings.Join(req.Platforms, ","), "--manifest", req.Tags[0]}
	} else {
		args = []string{"build"}
		if len(req.Platforms) == 1 {
			args = append(args, "--platform", req.Platforms[0])
		}
		for _, t := range req.Tags {
			args = append(args, "-t", t)
		}
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

func pullImageArgs(req image.PullRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("pull reference is required")
	}
	return []string{"pull", req.Reference}, nil
}

func pushImageArgs(req image.PushRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("push reference is required")
	}
	return []string{"push", req.Reference}, nil
}

func tagImageArgs(req image.TagRequest) ([]string, error) {
	if req.Source == "" {
		return nil, fmt.Errorf("tag source is required")
	}
	if req.Target == "" {
		return nil, fmt.Errorf("tag target is required")
	}
	return []string{"tag", req.Source, req.Target}, nil
}

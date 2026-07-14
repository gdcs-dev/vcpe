package hostnet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	lookupPath             = exec.LookPath
	podmanMachineInfoCheck = defaultPodmanMachineInfoCheck
	runtimeGOOS            = runtime.GOOS
)

type Intent struct {
	Role             string
	Bridge           string
	RequiresNAT      bool
	RequiresFirewall bool
}

type CapabilityReport struct {
	HasIPCmd       bool
	HasIPTablesCmd bool
	HasPodmanCmd   bool
	CanConfigure   bool
	Delegated      bool
}

type capabilityProvider func() CapabilityReport
type commandRunner func(context.Context, string, ...string) error

type Adapter struct {
	capabilities capabilityProvider
	runCmd       commandRunner
}

func New() Adapter {
	return Adapter{capabilities: detectCapabilities, runCmd: run}
}

func (a Adapter) runner() commandRunner {
	if a.runCmd != nil {
		return a.runCmd
	}
	return run
}

func (a Adapter) Preflight(intents []Intent) error {
	report := a.capabilities()
	if report.Delegated {
		if !report.HasPodmanCmd {
			return fmt.Errorf("host-network preflight failed: delegated mode requires podman command")
		}
		return nil
	}
	if !report.HasIPCmd {
		return fmt.Errorf("host-network preflight failed: missing ip command; install iproute2 on Linux or run through a delegated Podman machine host")
	}
	for _, intent := range intents {
		if intent.RequiresNAT || intent.RequiresFirewall {
			if !report.HasIPTablesCmd {
				return fmt.Errorf("host-network preflight failed for role %s: missing iptables command; install iptables or delegate host-network reconciliation to a supported Linux host", intent.Role)
			}
			if !report.CanConfigure {
				return fmt.Errorf("host-network preflight failed for role %s: insufficient privileges; run with elevated privileges or configure delegated host-network execution", intent.Role)
			}
		}
	}
	return nil
}

func (a Adapter) EnsureBridge(ctx context.Context, bridge string) error {
	if strings.TrimSpace(bridge) == "" {
		return fmt.Errorf("bridge name is required")
	}
	if err := a.runner()(ctx, "ip", "link", "show", bridge); err == nil {
		return nil
	}
	if err := a.runner()(ctx, "ip", "link", "add", bridge, "type", "bridge"); err != nil {
		return fmt.Errorf("ensure bridge %s: %w", bridge, err)
	}
	if err := a.runner()(ctx, "ip", "link", "set", bridge, "up"); err != nil {
		return fmt.Errorf("bring bridge %s up: %w", bridge, err)
	}
	return nil
}

func (a Adapter) EnsureNAT(ctx context.Context, sourceCIDR string) error {
	if strings.TrimSpace(sourceCIDR) == "" {
		return fmt.Errorf("nat source cidr is required")
	}
	if err := a.runner()(ctx, "iptables", "-t", "nat", "-C", "POSTROUTING", "-s", sourceCIDR, "-j", "MASQUERADE"); err == nil {
		return nil
	}
	if err := a.runner()(ctx, "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", sourceCIDR, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("ensure nat masquerade for %s: %w", sourceCIDR, err)
	}
	return nil
}

func (a Adapter) EnsureFirewallForward(ctx context.Context, bridge string) error {
	if strings.TrimSpace(bridge) == "" {
		return fmt.Errorf("bridge name is required")
	}
	if err := a.runner()(ctx, "iptables", "-C", "FORWARD", "-i", bridge, "-j", "ACCEPT"); err == nil {
		return nil
	}
	if err := a.runner()(ctx, "iptables", "-A", "FORWARD", "-i", bridge, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("ensure firewall forward for %s: %w", bridge, err)
	}
	return nil
}

func (a Adapter) Reconcile(ctx context.Context, intents []Intent, roleCIDRs map[string]string) error {
	for _, intent := range intents {
		if err := a.EnsureBridge(ctx, intent.Bridge); err != nil {
			return fmt.Errorf("reconcile role %s bridge %s: %w", intent.Role, intent.Bridge, err)
		}
		if intent.RequiresNAT {
			cidr := roleCIDRs[intent.Role]
			if strings.TrimSpace(cidr) == "" {
				return fmt.Errorf("reconcile role %s bridge %s: missing source CIDR for NAT", intent.Role, intent.Bridge)
			}
			if err := a.EnsureNAT(ctx, cidr); err != nil {
				return fmt.Errorf("reconcile role %s bridge %s: %w", intent.Role, intent.Bridge, err)
			}
		}
		if intent.RequiresFirewall {
			if err := a.EnsureFirewallForward(ctx, intent.Bridge); err != nil {
				return fmt.Errorf("reconcile role %s bridge %s: %w", intent.Role, intent.Bridge, err)
			}
		}
	}
	return nil
}

func detectCapabilities() CapabilityReport {
	_, hasIP := lookupPath("ip")
	_, hasIPTables := lookupPath("iptables")
	_, hasPodman := lookupPath("podman")
	delegated := delegatedHostnetMode(hasPodman == nil)
	canConfigure := os.Geteuid() == 0
	if delegated {
		canConfigure = true
	}
	return CapabilityReport{
		HasIPCmd:       hasIP == nil,
		HasIPTablesCmd: hasIPTables == nil,
		HasPodmanCmd:   hasPodman == nil,
		CanConfigure:   canConfigure,
		Delegated:      delegated,
	}
}

func run(ctx context.Context, name string, args ...string) error {
	_, hasPodman := lookupPath("podman")
	if delegatedHostnetMode(hasPodman == nil) {
		sshArgs := []string{"machine", "ssh", "--", name}
		sshArgs = append(sshArgs, args...)
		cmd := exec.CommandContext(ctx, "podman", sshArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("podman %s: %w (%s)", strings.Join(sshArgs, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func delegatedHostnetMode(hasPodman bool) bool {
	if os.Getenv("VCPE_HOSTNET_DELEGATED") == "1" {
		return true
	}
	if runtimeGOOS == "linux" {
		return false
	}
	if !hasPodman {
		return false
	}
	return podmanMachineInfoCheck() == nil
}

func defaultPodmanMachineInfoCheck() error {
	cmd := exec.Command("podman", "machine", "info")
	return cmd.Run()
}

// InterfaceInfo describes a network interface on the Podman host.
type InterfaceInfo struct {
	Name      string
	LinkType  string // "ether", "loopback", "bridge", "macvlan", etc.
	OperState string // "UP", "DOWN", "UNKNOWN"
	Addresses []string
}

// ListInterfaces returns physical network interfaces suitable for use as a
// macvlan/ipvlan parent. Loopback, virtual bridges, and macvlan sub-interfaces
// are filtered out. On macOS, the query is delegated to the Podman machine.
// Returns an empty slice (never an error) so callers can fall back gracefully.
func (a Adapter) ListInterfaces(ctx context.Context) []InterfaceInfo {
	var out []byte
	var err error
	if runtimeGOOS == "darwin" {
		cmd := exec.CommandContext(ctx, "podman", "machine", "ssh", "--", "ip", "-j", "link")
		out, err = cmd.Output()
	} else {
		cmd := exec.CommandContext(ctx, "ip", "-j", "link")
		out, err = cmd.Output()
	}
	if err != nil {
		return nil
	}

	var raw []struct {
		IfName    string `json:"ifname"`
		LinkType  string `json:"link_type"`
		OperState string `json:"operstate"`
		AddrInfo  []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
			Prefix int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	skip := func(name, linkType string) bool {
		if linkType == "loopback" || linkType == "macvlan" || linkType == "bridge" {
			return true
		}
		for _, prefix := range []string{"podman", "cni-", "veth", "br-", "docker", "virbr"} {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
		return false
	}

	result := make([]InterfaceInfo, 0, len(raw))
	for _, iface := range raw {
		if skip(iface.IfName, iface.LinkType) {
			continue
		}
		info := InterfaceInfo{
			Name:      iface.IfName,
			LinkType:  iface.LinkType,
			OperState: iface.OperState,
		}
		for _, a := range iface.AddrInfo {
			if a.Family == "inet" || a.Family == "inet6" {
				info.Addresses = append(info.Addresses, fmt.Sprintf("%s/%d", a.Local, a.Prefix))
			}
		}
		result = append(result, info)
	}
	return result
}

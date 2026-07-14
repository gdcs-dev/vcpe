package wizard

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/hostnet"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// NetworkEntry holds the resolved network values for use as defaults in the
// service config phase.
type NetworkEntry struct {
	CIDR      string
	Gateway   string
	PoolStart string
	PoolEnd   string
}

// AskNetworks interactively collects network declarations from the user.
// existing is non-nil in update mode and pre-fills the prompts.
// Returns the collected networks and a role→NetworkEntry lookup table for the
// service config phase.
func AskNetworks(ctx context.Context, r io.Reader, w io.Writer, existing []manifest.Network, ha *hostnet.Adapter) ([]manifest.Network, map[string]NetworkEntry) {
	nets := make([]manifest.Network, 0, max(len(existing), 2))
	lookup := map[string]NetworkEntry{}

	if len(existing) > 0 {
		// Update mode: edit each existing network in order.
		for _, n := range existing {
			edited := askOneNetwork(ctx, r, w, &n, ha)
			nets = append(nets, edited)
			if edited.IPv4 != nil {
				e := NetworkEntry{CIDR: edited.IPv4.CIDR, Gateway: edited.IPv4.Gateway}
				if edited.IPv4.Pool != nil {
					e.PoolStart = edited.IPv4.Pool.Start
					e.PoolEnd = edited.IPv4.Pool.End
				}
				lookup[edited.Role] = e
			}
		}
		fmt.Fprintln(w)
		if PromptBool(w, r, "Add another network?", false) {
			n := askOneNetwork(ctx, r, w, nil, ha)
			nets = append(nets, n)
			if n.IPv4 != nil {
				e := NetworkEntry{CIDR: n.IPv4.CIDR, Gateway: n.IPv4.Gateway}
				if n.IPv4.Pool != nil {
					e.PoolStart = n.IPv4.Pool.Start
					e.PoolEnd = n.IPv4.Pool.End
				}
				lookup[n.Role] = e
			}
		}
		return nets, lookup
	}

	// Create mode: collect networks until user says no.
	for {
		n := askOneNetwork(ctx, r, w, nil, ha)
		nets = append(nets, n)
		if n.IPv4 != nil {
			e := NetworkEntry{CIDR: n.IPv4.CIDR, Gateway: n.IPv4.Gateway}
			if n.IPv4.Pool != nil {
				e.PoolStart = n.IPv4.Pool.Start
				e.PoolEnd = n.IPv4.Pool.End
			}
			lookup[n.Role] = e
		}
		fmt.Fprintln(w)
		if !PromptBool(w, r, "Add another network?", len(nets) < 2) {
			break
		}
	}
	return nets, lookup
}

func askOneNetwork(ctx context.Context, r io.Reader, w io.Writer, existing *manifest.Network, ha *hostnet.Adapter) manifest.Network {
	var def manifest.Network
	if existing != nil {
		def = *existing
	}

	fmt.Fprintln(w, "\n─── Network ───")

	n := manifest.Network{}
	n.Role = Prompt(w, r, "Role (e.g. mgmt, wan, cm, lan-p1)", def.Role)

	drivers := []string{"bridge", "macvlan", "ipvlan"}
	defDriver := def.Driver
	if defDriver == "" {
		defDriver = "bridge"
	}
	driverIdx := 0
	for i, d := range drivers {
		if d == defDriver {
			driverIdx = i
			break
		}
	}
	n.Driver = PromptSelect(w, r, "Network driver", drivers, driverIdx)

	// Driver-specific options.
	if n.Driver == "macvlan" || n.Driver == "ipvlan" {
		existingParent := ""
		if existing != nil {
			existingParent = existing.DriverOptions["parent"]
		}
		parent := askParentInterface(ctx, r, w, existingParent, ha)
		n.DriverOptions = map[string]string{"parent": parent}
		if n.Driver == "ipvlan" {
			mode := Prompt(w, r, "IPVlan mode (l2/l3)", "l2")
			n.DriverOptions["mode"] = mode
		}
	} else {
		// Bridge: NAT and firewall options.
		defNAT := def.NAT
		defFW := def.Firewall
		n.NAT = PromptBool(w, r, "Enable NAT?", defNAT)
		n.Firewall = PromptBool(w, r, "Enable firewall forward?", defFW)
	}

	// Optional bridge name override.
	n.Bridge = Prompt(w, r, "Bridge name override (leave empty for default)", def.Bridge)

	// IPv4 address family.
	var defCIDR, defGW, defPS, defPE string
	if def.IPv4 != nil {
		defCIDR = def.IPv4.CIDR
		defGW = def.IPv4.Gateway
		if def.IPv4.Pool != nil {
			defPS = def.IPv4.Pool.Start
			defPE = def.IPv4.Pool.End
		}
	}
	if PromptBool(w, r, "Configure IPv4?", defCIDR != "") {
		cidr := Prompt(w, r, "CIDR (e.g. 10.7.200.0/24)", defCIDR)
		gw := Prompt(w, r, "Gateway (e.g. 10.7.200.1)", defGW)
		n.IPv4 = &manifest.AddressFamily{CIDR: cidr, Gateway: gw}
		if PromptBool(w, r, "Configure IP pool?", defPS != "") {
			ps := Prompt(w, r, "Pool start", defPS)
			pe := Prompt(w, r, "Pool end", defPE)
			n.IPv4.Pool = &manifest.Pool{Start: ps, End: pe}
		}
	}

	return n
}

func askParentInterface(ctx context.Context, r io.Reader, w io.Writer, existing string, ha *hostnet.Adapter) string {
	if ha == nil {
		return Prompt(w, r, "Parent interface name", existing)
	}

	fmt.Fprintln(w, "\nFetching interfaces from Podman host...")
	ifaces := ha.ListInterfaces(ctx)

	if len(ifaces) == 0 {
		fmt.Fprintln(w, "(Could not discover interfaces; enter manually)")
		return Prompt(w, r, "Parent interface name", existing)
	}

	fmt.Fprintln(w, "\nAvailable interfaces:")
	opts := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs := strings.Join(iface.Addresses, ", ")
		if addrs == "" {
			addrs = "<no IP>"
		}
		fmt.Fprintf(w, "  %-12s  %-8s  %s  %s\n", iface.Name, iface.OperState, iface.LinkType, addrs)
		opts = append(opts, iface.Name)
	}

	defIdx := 0
	for i, opt := range opts {
		if opt == existing {
			defIdx = i
			break
		}
	}
	return PromptSelect(w, r, "Select parent interface", opts, defIdx)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

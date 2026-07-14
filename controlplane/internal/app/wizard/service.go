package wizard

import (
	"fmt"
	"io"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	bngtype "github.com/gdcs-dev/vcpe/controlplane/internal/types/bng"
	gatewaytype "github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
	generictype "github.com/gdcs-dev/vcpe/controlplane/internal/types/genericcontainer"
	"gopkg.in/yaml.v3"
)

// AskServices interactively collects service declarations. existing is non-nil
// in update mode. types is the list of registered service type names.
func AskServices(r io.Reader, w io.Writer, existing []manifest.Service, nets map[string]NetworkEntry, types []string) []manifest.Service {
	svcs := make([]manifest.Service, 0, max(len(existing), 1))

	if len(existing) > 0 {
		for _, s := range existing {
			edited := askOneService(r, w, &s, nets, types, serviceNames(svcs))
			svcs = append(svcs, edited)
		}
		fmt.Fprintln(w)
		if PromptBool(w, r, "Add another service?", false) {
			s := askOneService(r, w, nil, nets, types, serviceNames(svcs))
			svcs = append(svcs, s)
		}
		return svcs
	}

	for {
		s := askOneService(r, w, nil, nets, types, serviceNames(svcs))
		svcs = append(svcs, s)
		fmt.Fprintln(w)
		if !PromptBool(w, r, "Add another service?", len(svcs) < 2) {
			break
		}
	}
	return svcs
}

func serviceNames(svcs []manifest.Service) []string {
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		names = append(names, s.Name)
	}
	return names
}

func askOneService(r io.Reader, w io.Writer, existing *manifest.Service, nets map[string]NetworkEntry, types []string, definedNames []string) manifest.Service {
	var def manifest.Service
	if existing != nil {
		def = *existing
	}

	fmt.Fprintln(w, "\n─── Service ───")

	s := manifest.Service{}
	s.Name = Prompt(w, r, "Service name", def.Name)

	defReplicas := def.Replicas
	if defReplicas == 0 {
		defReplicas = 1
	}
	replicasStr := Prompt(w, r, "Replicas", fmt.Sprintf("%d", defReplicas))
	fmt.Sscanf(replicasStr, "%d", &s.Replicas)
	if s.Replicas < 1 {
		s.Replicas = 1
	}

	defTypeIdx := 0
	for i, t := range types {
		if t == def.Type {
			defTypeIdx = i
			break
		}
	}
	s.Type = PromptSelect(w, r, "Service type", types, defTypeIdx)

	// Image.
	s.Image = askImage(r, w, def.Image, s.Type)

	// Interfaces.
	s.Interfaces = askInterfaces(r, w, def.Interfaces, nets)

	// DependsOn.
	if len(definedNames) > 0 {
		fmt.Fprintf(w, "\nAvailable services to depend on: %s\n", strings.Join(definedNames, ", "))
		if PromptBool(w, r, "Add dependency?", len(def.DependsOn) > 0) {
			dep := Prompt(w, r, "Depends on (comma-separated)", strings.Join(def.DependsOn, ", "))
			for _, d := range strings.Split(dep, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					s.DependsOn = append(s.DependsOn, d)
				}
			}
		}
	}

	// Type-specific config.
	s.Config = askConfig(r, w, s.Type, s.Interfaces, nets, def.Config)

	return s
}

func askImage(r io.Reader, w io.Writer, def manifest.Image, svcType string) manifest.Image {
	fmt.Fprintln(w)
	img := manifest.Image{}
	defRepo := def.Repository
	if defRepo == "" {
		defRepo = defaultRepo(svcType)
	}
	img.Repository = Prompt(w, r, "Image repository", defRepo)
	img.Tag = Prompt(w, r, "Image tag", orDefault(def.Tag, "dev"))
	img.PullPolicy = Prompt(w, r, "Pull policy (always-pull/build-if-missing/missing)", orDefault(def.PullPolicy, "always-pull"))
	img.BuildContext = Prompt(w, r, "Build context (leave empty to skip local build)", def.BuildContext)
	if img.BuildContext != "" {
		img.Containerfile = Prompt(w, r, "Containerfile path (leave empty for auto-detect)", def.Containerfile)
	}
	return img
}

func defaultRepo(svcType string) string {
	switch svcType {
	case "bng":
		return "ghcr.io/gdcs-dev/bng"
	case "gateway":
		return "ghcr.io/gdcs-dev/gateway"
	case "webpa":
		return "ghcr.io/gdcs-dev/webpa"
	case "event-sink":
		return "ghcr.io/gdcs-dev/event-sink"
	default:
		return ""
	}
}

func askInterfaces(r io.Reader, w io.Writer, existing []manifest.Interface, nets map[string]NetworkEntry) []manifest.Interface {
	fmt.Fprintln(w)
	roleNames := make([]string, 0, len(nets))
	for role := range nets {
		roleNames = append(roleNames, role)
	}
	if len(roleNames) > 0 {
		fmt.Fprintf(w, "Available network roles: %s\n", strings.Join(roleNames, ", "))
	}

	ifaces := make([]manifest.Interface, 0, max(len(existing), 1))
	if len(existing) > 0 {
		for _, iface := range existing {
			edited := askOneInterface(r, w, &iface)
			ifaces = append(ifaces, edited)
		}
		if PromptBool(w, r, "Add another interface?", false) {
			ifaces = append(ifaces, askOneInterface(r, w, nil))
		}
		return ifaces
	}
	for {
		ifaces = append(ifaces, askOneInterface(r, w, nil))
		if !PromptBool(w, r, "Add another interface?", len(ifaces) < 1) {
			break
		}
	}
	return ifaces
}

func askOneInterface(r io.Reader, w io.Writer, existing *manifest.Interface) manifest.Interface {
	var def manifest.Interface
	if existing != nil {
		def = *existing
	}
	iface := manifest.Interface{}
	iface.Role = Prompt(w, r, "  Interface role", def.Role)
	iface.IPv4 = Prompt(w, r, "  Static IPv4 (leave empty for IPAM)", def.IPv4)
	iface.DefaultRoute = PromptBool(w, r, "  Default route?", def.DefaultRoute)
	return iface
}

// askConfig dispatches to a type-specific config asker and returns a yaml.Node.
func askConfig(r io.Reader, w io.Writer, svcType string, ifaces []manifest.Interface, nets map[string]NetworkEntry, existing yaml.Node) yaml.Node {
	fmt.Fprintf(w, "\n─── %s config ───\n", svcType)
	var cfg interface{}

	switch svcType {
	case "bng":
		cfg = askBNGConfig(r, w, ifaces, nets)
	case "gateway":
		cfg = askGatewayConfig(r, w, ifaces, nets)
	case "generic-container":
		cfg = askGenericConfig(r, w, existing)
	default:
		// No config needed for this type.
		fmt.Fprintf(w, "(no configuration required for type %q)\n", svcType)
		return yaml.Node{}
	}

	// Marshal to yaml.Node for storage in the manifest.
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return yaml.Node{}
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return yaml.Node{}
	}
	// yaml.Unmarshal wraps in a document node; unwrap.
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return *node.Content[0]
	}
	return node
}

func askBNGConfig(r io.Reader, w io.Writer, ifaces []manifest.Interface, nets map[string]NetworkEntry) bngtype.Config {
	var cfg bngtype.Config
	for _, iface := range ifaces {
		net, ok := nets[iface.Role]
		if !ok {
			continue
		}
		if !PromptBool(w, r, fmt.Sprintf("Configure DHCP4 for role %q?", iface.Role), true) {
			continue
		}
		dhcp4 := &bngtype.DHCP4{
			Subnet:       Prompt(w, r, "  DHCP4 subnet", net.CIDR),
			LeaseSeconds: 3600,
		}
		rangeStart := Prompt(w, r, "  Range start", net.PoolStart)
		rangeEnd := Prompt(w, r, "  Range end", net.PoolEnd)
		if rangeStart != "" && rangeEnd != "" {
			dhcp4.Ranges = []bngtype.Range{{Start: rangeStart, End: rangeEnd}}
		}
		routers := Prompt(w, r, "  Routers option", net.Gateway)
		if routers != "" {
			dhcp4.Options = map[string]string{"routers": routers}
		}
		cfg.Access = append(cfg.Access, bngtype.AccessSegment{
			Role:  iface.Role,
			DHCP4: dhcp4,
		})
	}
	return cfg
}

func askGatewayConfig(r io.Reader, w io.Writer, ifaces []manifest.Interface, nets map[string]NetworkEntry) gatewaytype.Config {
	cfg := gatewaytype.Config{}
	// Find the first LAN network for defaults.
	for _, iface := range ifaces {
		if net, ok := nets[iface.Role]; ok && strings.HasPrefix(iface.Role, "lan") {
			cfg.LAN.IPv4 = Prompt(w, r, "LAN IPv4 CIDR (e.g. 192.168.10.1/24)", net.Gateway+"/24")
			cfg.LAN.DHCPStart = Prompt(w, r, "LAN DHCP start", net.PoolStart)
			cfg.LAN.DHCPEnd = Prompt(w, r, "LAN DHCP end", net.PoolEnd)
			break
		}
	}
	if cfg.LAN.IPv4 == "" {
		cfg.LAN.IPv4 = Prompt(w, r, "LAN IPv4 CIDR", "")
		cfg.LAN.DHCPStart = Prompt(w, r, "LAN DHCP start", "")
		cfg.LAN.DHCPEnd = Prompt(w, r, "LAN DHCP end", "")
	}
	return cfg
}

func askGenericConfig(r io.Reader, w io.Writer, existing yaml.Node) generictype.Config {
	cfg := generictype.Config{}
	if existing.Kind != 0 {
		_ = existing.Decode(&cfg)
	}
	cmdStr := strings.Join(cfg.Command, " ")
	cmdStr = Prompt(w, r, "Command (space-separated, leave empty for default)", cmdStr)
	if cmdStr != "" {
		cfg.Command = strings.Fields(cmdStr)
	}
	return cfg
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

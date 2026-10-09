package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// validDoc returns a minimal, well-formed v1 document that individual tests
// mutate to exercise a single failure mode.
func validDoc() Document {
	return Document{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   Metadata{Name: "edge"},
		Spec: Spec{
			Networks: []Network{
				{Role: "wan", IPv4: &AddressFamily{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"}},
				{Role: "lan", IPv4: &AddressFamily{CIDR: "10.7.210.0/24"}},
			},
			Services: []Service{
				{
					Name:     "bng",
					Type:     "bng",
					Replicas: 1,
					Image:    Image{Repository: "ghcr.io/gdcs-dev/bng", Tag: "dev"},
					Interfaces: []Interface{
						{Role: "wan", IPv4: "10.7.200.2", DefaultRoute: true, Addressing: AddressingStatic},
						{Role: "lan", IPv4: "10.7.210.2", Addressing: AddressingStatic},
					},
				},
			},
		},
	}
}

func TestValidateAcceptsWellFormedDocument(t *testing.T) {
	if err := Validate(validDoc()); err != nil {
		t.Fatalf("expected valid document, got %v", err)
	}
}

// TestLoadRejectsHealthUpstreamAsUnknownField proves the removed
// services[].interfaces[].healthUpstream field has no alias or compatibility
// path: strict decoding rejects it outright.
func TestLoadRejectsHealthUpstreamAsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  networks:\n    - role: wan\n      ipamDriver: none\n      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1 }\n  services:\n    - name: gateway\n      type: gateway\n      replicas: 1\n      image: { repository: ghcr.io/gdcs-dev/gateway, tag: dev }\n      interfaces:\n        - { role: wan, device: erouter0, ipv4: \"10.7.200.10\", addressing: static, healthUpstream: true }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "healthUpstream") {
		t.Fatalf("Load() error = %v, want an unknown-field error mentioning healthUpstream", err)
	}
}

// TestLoadAcceptsSelfAddressedServiceWithoutHealthAnnotation proves a
// self-addressed health-capable service (no Podman-managed network) needs no
// replacement annotation: health publication is automatic and control-plane
// owned, not manifest-declared.
func TestLoadAcceptsSelfAddressedServiceWithoutHealthAnnotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  networks:\n    - role: wan\n      ipamDriver: none\n      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1 }\n  services:\n    - name: gateway\n      type: gateway\n      replicas: 1\n      image: { repository: ghcr.io/gdcs-dev/gateway, tag: dev }\n      interfaces:\n        - { role: wan, device: erouter0, ipv4: \"10.7.200.10\", addressing: static }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want a self-addressed service to need no health annotation", err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsupportedAPIVersion(t *testing.T) {
	doc := validDoc()
	doc.APIVersion = "vcpe.dev/v0"
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "unsupported apiVersion") {
		t.Fatalf("expected unsupported apiVersion error, got %v", err)
	}
}

func TestValidateRejectsUnsupportedKind(t *testing.T) {
	doc := validDoc()
	doc.Kind = "Service"
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestValidateRequiresMetadataName(t *testing.T) {
	doc := validDoc()
	doc.Metadata.Name = ""
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("expected metadata.name error, got %v", err)
	}
}

func TestValidateRejectsInterfaceRoleWithoutNetwork(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces = append(doc.Spec.Services[0].Interfaces, Interface{Role: "mgmt"})
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "unknown network role") {
		t.Fatalf("expected unknown network role error, got %v", err)
	}
}

func TestValidateRejectsDuplicateServiceNames(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services = append(doc.Spec.Services, doc.Spec.Services[0])
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "duplicate service name") {
		t.Fatalf("expected duplicate service name error, got %v", err)
	}
}

func TestValidateRejectsDependsOnCycle(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].DependsOn = []string{"webpa"}
	doc.Spec.Services = append(doc.Spec.Services, Service{
		Name:      "webpa",
		Type:      "webpa",
		Replicas:  1,
		Image:     Image{Repository: "ghcr.io/gdcs-dev/webpa"},
		DependsOn: []string{"bng"},
	})
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected dependsOn cycle error, got %v", err)
	}
}

func TestValidateRejectsUnknownDependsOn(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].DependsOn = []string{"ghost"}
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("expected unknown dependsOn error, got %v", err)
	}
}

func TestValidateRejectsExplicitAddressOutsideCIDR(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].IPv4 = "192.168.0.5"
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "outside the CIDR") {
		t.Fatalf("expected out-of-CIDR error, got %v", err)
	}
}

func TestValidateRejectsReplicasOverMax(t *testing.T) {
	doc := validDoc()
	doc.Spec.MaxReplicasPerService = 1
	doc.Spec.Services[0].Replicas = 2
	// Explicit addresses are invalid with replicas>1; clear them so we isolate
	// the cap check.
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	doc.Spec.Services[0].Interfaces[0].Addressing = ""
	doc.Spec.Services[0].Interfaces[1].IPv4 = ""
	doc.Spec.Services[0].Interfaces[1].Addressing = ""
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "maxReplicasPerService") {
		t.Fatalf("expected replicas-over-max error, got %v", err)
	}
}

func TestValidateRejectsMultipleDefaultRoutes(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[1].DefaultRoute = true
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "default route") {
		t.Fatalf("expected multiple default route error, got %v", err)
	}
}

func TestValidateRejectsExplicitAddressWithMultipleReplicas(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Replicas = 2
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "replicas") {
		t.Fatalf("expected explicit-address-with-replicas error, got %v", err)
	}
}

func TestValidateRejectsGatewayOutsideCIDR(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks[0].IPv4.Gateway = "10.8.0.1"
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Fatalf("expected gateway-outside-cidr error, got %v", err)
	}
}

func TestValidateAcceptsIPv6OnlyNetwork(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks = []Network{
		{Role: "wan", IPv6: &AddressFamily{CIDR: "2001:dae:7:1::/64"}},
		{Role: "lan", IPv4: &AddressFamily{CIDR: "10.7.210.0/24"}},
	}
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	doc.Spec.Services[0].Interfaces[0].IPv6 = "2001:dae:7:1::2"
	if err := Validate(doc); err != nil {
		t.Fatalf("expected IPv6-only network to validate, got %v", err)
	}
}

func TestValidateAcceptsMacvlanWithParent(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks[0] = Network{
		Role:          "wan",
		Driver:        "macvlan",
		DriverOptions: map[string]string{"parent": "eth0"},
		IPv4:          &AddressFamily{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("expected macvlan with parent to validate, got %v", err)
	}
}

func TestValidateRejectsMacvlanWithoutParent(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks[0] = Network{
		Role:   "wan",
		Driver: "macvlan",
		IPv4:   &AddressFamily{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"},
	}
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected missing parent error, got %v", err)
	}
}

func TestValidateRejectsNATOnMacvlan(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks[0] = Network{
		Role:          "wan",
		Driver:        "macvlan",
		NAT:           true,
		DriverOptions: map[string]string{"parent": "eth0"},
		IPv4:          &AddressFamily{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"},
	}
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "nat") {
		t.Fatalf("expected nat-on-macvlan error, got %v", err)
	}
}

func TestValidateRejectsFirewallOnIPVlan(t *testing.T) {
	doc := validDoc()
	doc.Spec.Networks[0] = Network{
		Role:          "wan",
		Driver:        "ipvlan",
		Firewall:      true,
		DriverOptions: map[string]string{"parent": "eth0"},
		IPv4:          &AddressFamily{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"},
	}
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "firewall") {
		t.Fatalf("expected firewall-on-ipvlan error, got %v", err)
	}
}

func TestValidateAcceptsDefaultAddressingAsDHCP(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	doc.Spec.Services[0].Interfaces[0].Addressing = ""
	if err := Validate(doc); err != nil {
		t.Fatalf("expected omitted addressing to default to dhcp and validate, got %v", err)
	}
}

func TestValidateRejectsStaticAddressingWithoutAddress(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "declares no ipv4/ipv6 address") {
		t.Fatalf("expected static-without-address error, got %v", err)
	}
}

func TestValidateRejectsDHCPAddressingWithAddress(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].Addressing = AddressingDHCP
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "addressing is \"dhcp\"") {
		t.Fatalf("expected dhcp-with-address error, got %v", err)
	}
}

func TestValidateRejectsDefaultAddressingWithAddress(t *testing.T) {
	// validDoc's interfaces already set addressing: static + ipv4; flip one to
	// omit addressing entirely while keeping its ipv4 to prove the default of
	// dhcp still conflicts with an explicit address.
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].Addressing = ""
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "addressing is \"dhcp\"") {
		t.Fatalf("expected default-dhcp-with-address error, got %v", err)
	}
}

func TestValidateRejectsInvalidAddressingValue(t *testing.T) {
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].Addressing = "bogus"
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "invalid addressing") {
		t.Fatalf("expected invalid-addressing error, got %v", err)
	}
}

func TestValidateIgnoresAddressingOnBridgeEnslavedInterface(t *testing.T) {
	doc := validDoc()
	// A bridge-enslaved interface with a contradictory static+no-address
	// combination is still accepted: addressing is ignored entirely when
	// Bridge is set.
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	doc.Spec.Services[0].Interfaces[0].Bridge = "brlan0"
	if err := Validate(doc); err != nil {
		t.Fatalf("expected bridge-enslaved interface to skip addressing validation, got %v", err)
	}
}

func TestValidateDoesNotGuardDHCPServerRoleAddressing(t *testing.T) {
	// There is deliberately no cross-check against a service's own DHCP-server
	// config (e.g. bng serving DHCP on the same role it's set to dhcp on):
	// this must pass validation and is left to fail at runtime if misused.
	doc := validDoc()
	doc.Spec.Services[0].Interfaces[0].IPv4 = ""
	doc.Spec.Services[0].Interfaces[0].Addressing = AddressingDHCP
	if err := Validate(doc); err != nil {
		t.Fatalf("expected no addressing guard for DHCP-server-role services, got %v", err)
	}
}

func wirelessDoc() Document {
	doc := validDoc()
	doc.Spec.WirelessMedia = []WirelessMedium{{Name: "rf24", Band: "2.4ghz", Channel: 1, WidthMHz: 20}}
	doc.Spec.WirelessNetworks = []WirelessNetwork{{Name: "home", SSID: "vcpe-lab", Security: WirelessOpen}}
	doc.Spec.Services[0].Bridges = []BridgeSpec{{Name: "brlan0"}}
	doc.Spec.Services[0].Radios = []Radio{{Name: "home-ap", Medium: "rf24", Device: "wlan0", Mode: RadioModeAP, VAPs: []VAP{{Slot: 0, Network: "home", Bridge: "brlan0"}}}}
	doc.Spec.Services = append(doc.Spec.Services, Service{
		Name:     "station",
		Type:     "generic-container",
		Replicas: 1,
		Image:    Image{Repository: "wireless-client"},
		Radios:   []Radio{{Name: "home-station", Medium: "rf24", Network: "home", Device: "wlan0", Mode: RadioModeStation, Addressing: AddressingDHCP}},
	})
	return doc
}

func meshDoc() Document {
	doc := validDoc()
	doc.Spec.WirelessMedia = []WirelessMedium{
		{Name: "backhaul", Band: "5ghz", Channel: 36, WidthMHz: 20},
		{Name: "access", Band: "5ghz", Channel: 40, WidthMHz: 20},
		{Name: "remote-access", Band: "5ghz", Channel: 40, WidthMHz: 20},
	}
	doc.Spec.WirelessNetworks = []WirelessNetwork{{Name: "home", SSID: "home", Security: WirelessOpen}}
	doc.Spec.Secrets = []SecretRef{{Name: "mesh-key", Provider: "env", Key: "VCPE_MESH_KEY"}}
	doc.Spec.Services = nil
	for _, name := range []string{"root", "remote"} {
		bridge := BridgeSpec{Name: "brlan0"}
		if name == "root" {
			bridge.IPv4, bridge.DHCPStart, bridge.DHCPEnd = "10.0.0.1/24", "10.0.0.100", "10.0.0.200"
		}
		accessMedium := "access"
		if name == "remote" {
			accessMedium = "remote-access"
		}
		doc.Spec.Services = append(doc.Spec.Services, Service{
			Name: name, Type: "gateway", Replicas: 1, Image: Image{Repository: "gateway"},
			Bridges: []BridgeSpec{bridge},
			Radios: []Radio{{Name: "backhaul", Medium: "backhaul", Device: "mesh0", Mode: RadioModeMesh,
				Mesh: &Mesh{ID: "lab-mesh", Bridge: "brlan0", SAESecretRef: "mesh-key"}},
				{Name: "access", Medium: accessMedium, Device: "wlan0", Mode: RadioModeAP,
					VAPs: []VAP{{Slot: 0, Network: "home", Bridge: "brlan0"}}}},
		})
	}
	return doc
}

func TestLoadAndValidateMeshPair(t *testing.T) {
	data, err := yaml.Marshal(meshDoc())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("mesh pair should validate: %v", err)
	}
	if doc.Spec.Services[1].Radios[0].Mesh.SAESecretRef != "mesh-key" {
		t.Fatalf("mesh contract lost in parsing: %+v", doc.Spec.Services[1].Radios[0].Mesh)
	}
}

func TestValidateMeshContracts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{"missing peer", func(doc *Document) { doc.Spec.Services = doc.Spec.Services[:1] }, "no matching peer"},
		{"different id", func(doc *Document) { doc.Spec.Services[1].Radios[0].Mesh.ID = "other-mesh" }, "no matching peer"},
		{"different medium", func(doc *Document) {
			doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, WirelessMedium{Name: "other", Band: "5ghz", Channel: 40, WidthMHz: 20})
			doc.Spec.Services[1].Radios[0].Medium = "other"
		}, "inconsistent medium"},
		{"different secret", func(doc *Document) {
			doc.Spec.Secrets = append(doc.Spec.Secrets, SecretRef{Name: "other-key", Provider: "env", Key: "VCPE_OTHER_KEY"})
			doc.Spec.Services[1].Radios[0].Mesh.SAESecretRef = "other-key"
		}, "inconsistent medium or SAE secret"},
		{"missing mesh block", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh = nil }, "requires mesh"},
		{"missing bridge", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh.Bridge = "absent" }, "undeclared bridge"},
		{"missing credential", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh.SAESecretRef = "absent" }, "unknown SAE secret"},
		{"empty id", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh.ID = "" }, "id must be"},
		{"newline in id", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh.ID = "mesh\nunsafe" }, "printable"},
		{"control byte in id", func(doc *Document) { doc.Spec.Services[0].Radios[0].Mesh.ID = "mesh\x01unsafe" }, "printable"},
		{"non-Gateway", func(doc *Document) { doc.Spec.Services[0].Type = "generic-container" }, "requires a gateway"},
		{"mesh VAP", func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs = []VAP{{Slot: 0}} }, "cannot declare"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := meshDoc()
			test.edit(&doc)
			if err := Validate(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateMeshTopology(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{"two DHCP owners", func(doc *Document) { doc.Spec.Services[1].Bridges[0] = doc.Spec.Services[0].Bridges[0] }, "one"},
		{"no DHCP owner", func(doc *Document) { doc.Spec.Services[0].Bridges[0] = BridgeSpec{Name: "brlan0"} }, "one"},
		{"remote address", func(doc *Document) { doc.Spec.Services[1].Bridges[0].IPv4 = "10.0.0.2/24" }, "both DHCP bounds"},
		{"missing access AP", func(doc *Document) { doc.Spec.Services[1].Radios = doc.Spec.Services[1].Radios[:1] }, "access AP"},
		{"AP on other bridge", func(doc *Document) {
			doc.Spec.Services[1].Bridges = append(doc.Spec.Services[1].Bridges, BridgeSpec{Name: "other"})
			doc.Spec.Services[1].Radios[1].VAPs[0].Bridge = "other"
		}, "access AP"},
		{"extra gateway", func(doc *Document) {
			doc.Spec.Services = append(doc.Spec.Services, Service{Name: "third", Type: "gateway", Replicas: 1})
		}, "two"},
		{"multiple replicas", func(doc *Document) { doc.Spec.Services[1].Replicas = 2 }, "single-replica"},
		{"Podman LAN bypass", func(doc *Document) {
			doc.Spec.Services[1].Interfaces = []Interface{{Role: "lan", Bridge: "brlan0"}}
		}, "Podman"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := meshDoc()
			test.edit(&doc)
			if err := Validate(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseRejectsUnknownMeshField(t *testing.T) {
	data := []byte("apiVersion: vcpe.dev/v1\nkind: Deployment\nspec:\n  services:\n    - radios:\n        - mesh: {passphrase: inline}\n")
	if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("Parse() error = %v, want unknown mesh field", err)
	}
}

func TestLoadAndValidateWirelessMedia(t *testing.T) {
	tests := []struct {
		name    string
		medium  WirelessMedium
		wantErr string
	}{
		{name: "2.4 GHz", medium: WirelessMedium{Name: "rf24", Band: "2.4ghz", Channel: 14, WidthMHz: 20}},
		{name: "5 GHz", medium: WirelessMedium{Name: "rf5", Band: "5ghz", Channel: 149, WidthMHz: 20}},
		{name: "6 GHz PSC", medium: WirelessMedium{Name: "rf6", Band: "6ghz", Channel: 229, WidthMHz: 20}},
		{name: "invalid band", medium: WirelessMedium{Name: "rf6", Band: "7ghz", Channel: 5, WidthMHz: 20}, wantErr: "band"},
		{name: "DFS channel", medium: WirelessMedium{Name: "rf5", Band: "5ghz", Channel: 52, WidthMHz: 20}, wantErr: "channel"},
		{name: "non-PSC channel", medium: WirelessMedium{Name: "rf6", Band: "6ghz", Channel: 9, WidthMHz: 20}, wantErr: "channel"},
		{name: "wide channel", medium: WirelessMedium{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 40}, wantErr: "widthMHz"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := validDoc()
			doc.Spec.WirelessMedia = []WirelessMedium{test.medium}
			data, err := yaml.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := Parse(data)
			if err == nil {
				err = Validate(loaded)
			}
			if test.wantErr == "" && err != nil {
				t.Fatalf("medium %+v: %v", test.medium, err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("medium %+v: error = %v, want %q", test.medium, err, test.wantErr)
			}
		})
	}
}

func TestStrictLoadRejectsLegacyWirelessFields(t *testing.T) {
	base := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: edge}\nspec:\n  networks: [{role: mgmt}]\n  wirelessMedia: [{name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}]\n  wirelessNetworks: [{name: home, ssid: vcpe-lab, security: open}]\n  services:\n    - name: gateway\n      type: generic-container\n      image: {repository: example/gateway}\n      bridges: [{name: brlan0}]\n      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]\n"
	for _, test := range []struct {
		name, old, legacy, field string
	}{
		{"network channel", "ssid: vcpe-lab", "ssid: vcpe-lab, channel: 1", "channel"},
		{"AP bridge", "mode: ap, vaps:", "mode: ap, bridge: brlan0, vaps:", "bridge"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse([]byte(strings.Replace(base, test.old, test.legacy, 1))); err == nil || !strings.Contains(err.Error(), "field "+test.field+" not found") {
				t.Fatalf("legacy field %s: parse error = %v", test.field, err)
			}
		})
	}
}

func mediaRadioDoc() Document {
	doc := wirelessDoc()
	doc.Spec.WirelessMedia = []WirelessMedium{{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20}}
	doc.Spec.WirelessNetworks[0] = WirelessNetwork{Name: "home", SSID: "vcpe-lab", Security: WirelessWPA3Personal, PassphraseSecretRef: "home-wifi"}
	doc.Spec.Secrets = []SecretRef{{Name: "home-wifi", Provider: "env", Key: "VCPE_HOME_WIFI"}}
	doc.Spec.Services[0].Radios[0] = Radio{Name: "home-ap", Medium: "rf6", Device: "wlan0", Mode: RadioModeAP, VAPs: []VAP{{Slot: 0, Network: "home", Bridge: "brlan0"}}}
	doc.Spec.Services[1].Radios[0] = Radio{Name: "home-station", Medium: "rf6", Network: "home", Device: "wlan0", Mode: RadioModeStation}
	return doc
}

func TestLoadAndValidateMediaRadioAttachments(t *testing.T) {
	doc := mediaRadioDoc()
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(loaded); err != nil {
		t.Fatalf("media-based attachments should validate: %v", err)
	}
}

func TestLoadAndValidateTriBandEightVAPs(t *testing.T) {
	doc := mediaRadioDoc()
	doc.Spec.WirelessMedia = []WirelessMedium{
		{Name: "rf24", Band: "2.4ghz", Channel: 1, WidthMHz: 20},
		{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20},
		{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20},
	}
	doc.Spec.Services[0].Radios = nil
	for slot := 1; slot < 8; slot++ {
		profile := fmt.Sprintf("guest-%d", slot)
		bridge := fmt.Sprintf("brguest%d", slot)
		doc.Spec.WirelessNetworks = append(doc.Spec.WirelessNetworks, WirelessNetwork{
			Name: profile, SSID: profile, Security: WirelessWPA3Personal, PassphraseSecretRef: "home-wifi",
		})
		doc.Spec.Services[0].Bridges = append(doc.Spec.Services[0].Bridges, BridgeSpec{Name: bridge})
	}
	for radioIndex, medium := range doc.Spec.WirelessMedia {
		radio := Radio{Name: fmt.Sprintf("ap-%d", radioIndex), Medium: medium.Name, Device: fmt.Sprintf("wlan%d", radioIndex), Mode: RadioModeAP}
		radio.VAPs = append(radio.VAPs, VAP{Slot: 0, Network: "home", Bridge: "brlan0"})
		for slot := 1; slot < 8; slot++ {
			radio.VAPs = append(radio.VAPs, VAP{Slot: slot, Network: fmt.Sprintf("guest-%d", slot), Bridge: fmt.Sprintf("brguest%d", slot)})
		}
		doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, radio)
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Spec.Services[0].Radios) != 3 || len(parsed.Spec.Services[0].Radios[2].VAPs) != 8 {
		t.Fatalf("tri-band VAP declarations lost in strict loading: %#v", parsed.Spec.Services[0].Radios)
	}
}

func TestValidateMediaRadioTopology(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{name: "duplicate medium", edit: func(doc *Document) {
			doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, doc.Spec.WirelessMedia[0])
		}, want: "duplicate wireless medium"},
		{name: "duplicate profile", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks = append(doc.Spec.WirelessNetworks, doc.Spec.WirelessNetworks[0])
		}, want: "duplicate wireless network"},
		{name: "unknown medium", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].Medium = "missing" }, want: "unknown wireless medium"},
		{name: "no vaps", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs = nil }, want: "1 through 8 vaps"},
		{name: "too many vaps", edit: func(doc *Document) { radio := &doc.Spec.Services[0].Radios[0]; radio.VAPs = make([]VAP, 9) }, want: "1 through 8 vaps"},
		{name: "missing slot zero", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Slot = 1 }, want: "requires VAP slot 0"},
		{name: "invalid slot", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Slot = 8 }, want: "invalid VAP slot"},
		{name: "duplicate slot", edit: func(doc *Document) {
			radio := &doc.Spec.Services[0].Radios[0]
			radio.VAPs = append(radio.VAPs, radio.VAPs[0])
		}, want: "duplicate VAP slot"},
		{name: "unknown profile", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Network = "missing" }, want: "unknown wireless network"},
		{name: "duplicate profile in AP", edit: func(doc *Document) {
			radio := &doc.Spec.Services[0].Radios[0]
			radio.VAPs = append(radio.VAPs, VAP{Slot: 1, Network: "home", Bridge: "brlan0"})
		}, want: "repeats wireless network"},
		{name: "undeclared bridge", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Bridge = "missing" }, want: "undeclared bridge"},
		{name: "AP legacy network", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].Network = "home" }, want: "only in vaps"},
		{name: "station with vaps", edit: func(doc *Document) {
			doc.Spec.Services[1].Radios[0].VAPs = []VAP{{Slot: 0, Network: "home", Bridge: "brlan0"}}
		}, want: "cannot declare vaps"},
		{name: "station without profile", edit: func(doc *Document) { doc.Spec.Services[1].Radios[0].Network = "missing" }, want: "unknown wireless network"},
		{name: "6 GHz open AP", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks[0].Security = WirelessOpen
			doc.Spec.WirelessNetworks[0].PassphraseSecretRef = ""
		}, want: "requires wpa3-personal"},
		{name: "6 GHz WPA2 station", edit: func(doc *Document) {
			doc.Spec.Services[0].Radios = nil
			doc.Spec.WirelessNetworks[0].Security = WirelessWPA2Personal
		}, want: "requires wpa3-personal"},
		{name: "station without matching AP", edit: func(doc *Document) { doc.Spec.Services[0].Radios = nil }, want: "exactly one AP"},
		{name: "duplicate AP pair", edit: func(doc *Document) {
			radio := doc.Spec.Services[0].Radios[0]
			radio.Name = "other"
			radio.Device = "wlan1"
			doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, radio)
		}, want: "at most one"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := mediaRadioDoc()
			test.edit(&doc)
			err := Validate(doc)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateMirroredProfileAcrossMedia(t *testing.T) {
	doc := mediaRadioDoc()
	doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, WirelessMedium{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20})
	secondAP := doc.Spec.Services[0].Radios[0]
	secondAP.Name = "other-ap"
	secondAP.Device = "wlan1"
	secondAP.Medium = "rf5"
	doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, secondAP)
	if err := Validate(doc); err != nil {
		t.Fatalf("same profile on distinct media must be allowed: %v", err)
	}
	secondStation := doc.Spec.Services[1].Radios[0]
	secondStation.Name = "other-station"
	secondStation.Device = "wlan1"
	secondStation.Medium = "rf5"
	doc.Spec.Services[1].Radios = append(doc.Spec.Services[1].Radios, secondStation)
	if err := Validate(doc); err != nil {
		t.Fatalf("stations must match only their medium/profile AP: %v", err)
	}
}

func TestValidateRoamingStationCandidates(t *testing.T) {
	doc := wirelessDoc()
	secondAP := doc.Spec.Services[0].Radios[0]
	secondAP.Name, secondAP.Device = "other-ap", "wlan1"
	doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, secondAP)
	station := &doc.Spec.Services[1].Radios[0]
	station.Medium = ""
	station.Roaming = &Roaming{Media: []string{"rf24"}}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(parsed); err != nil {
		t.Fatalf("two same-medium APs should be eligible: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*Document)
		want string
	}{
		{"fixed and roaming", func(doc *Document) { doc.Spec.Services[1].Radios[0].Medium = "rf24" }, "cannot combine"},
		{"one AP", func(doc *Document) { doc.Spec.Services[0].Radios = doc.Spec.Services[0].Radios[:1] }, "at least two"},
		{"unknown medium", func(doc *Document) { doc.Spec.Services[1].Radios[0].Roaming.Media = []string{"absent"} }, "unknown wireless medium"},
		{"duplicate medium", func(doc *Document) { doc.Spec.Services[1].Radios[0].Roaming.Media = []string{"rf24", "rf24"} }, "duplicate roaming medium"},
		{"empty media", func(doc *Document) { doc.Spec.Services[1].Radios[0].Roaming.Media = nil }, "roaming.media"},
		{"fixed client on shared AP pair", func(doc *Document) {
			doc.Spec.Services[1].Radios = append(doc.Spec.Services[1].Radios, Radio{Name: "fixed", Mode: RadioModeStation, Device: "wlan1", Medium: "rf24", Network: "home"})
		}, "at most one"},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := doc
			invalid.Spec.Services = append([]Service(nil), doc.Spec.Services...)
			invalid.Spec.Services[0].Radios = append([]Radio(nil), doc.Spec.Services[0].Radios...)
			invalid.Spec.Services[1].Radios = append([]Radio(nil), doc.Spec.Services[1].Radios...)
			invalid.Spec.Services[1].Radios[0].Roaming = &Roaming{Media: append([]string(nil), station.Roaming.Media...)}
			test.edit(&invalid)
			if err := Validate(invalid); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func roamingScenarioDoc() Document {
	doc := wirelessDoc()
	other := doc.Spec.Services[0].Radios[0]
	other.Name, other.Device = "other-ap", "wlan1"
	doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, other)
	doc.Spec.Services[1].Radios[0].Medium = ""
	doc.Spec.Services[1].Radios[0].Roaming = &Roaming{Media: []string{"rf24"}}
	slot, first, next, strong, weak := 0, 0, 1000, 35, -100
	initial := VAPReference{Service: "bng", Replica: 1, Radio: "home-ap", Slot: &slot}
	final := VAPReference{Service: "bng", Replica: 1, Radio: "other-ap", Slot: &slot}
	doc.Spec.WirelessScenarios = []WirelessScenario{{
		Name:       "crossover",
		Station:    RadioReference{Service: "station", Replica: 1, Radio: "home-station"},
		APs:        []VAPReference{initial, final},
		Steps:      []RFStep{{AtMs: &first, AP: initial, SNRDb: &strong}, {AtMs: &next, AP: initial, SNRDb: &weak}, {AtMs: &next, AP: final, SNRDb: &strong}},
		Assertions: RoamAssertions{InitialAP: initial, FinalAP: final, SameIPv4: true, MaxRoamMs: 5000, MaxGapMs: 3000},
	}}
	return doc
}

func TestLoadAndValidateWirelessScenario(t *testing.T) {
	data, err := yaml.Marshal(roamingScenarioDoc())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(parsed); err != nil {
		t.Fatalf("valid roaming scenario: %v", err)
	}
	if parsed.Spec.WirelessScenarios[0].Steps[2].AP.Radio != "other-ap" {
		t.Fatalf("scenario endpoint lost in parsing: %+v", parsed.Spec.WirelessScenarios)
	}
}

func TestValidateWirelessScenarioReferencesAndBounds(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{"duplicate name", func(doc *Document) {
			doc.Spec.WirelessScenarios = append(doc.Spec.WirelessScenarios, doc.Spec.WirelessScenarios[0])
		}, "duplicate"},
		{"unknown station", func(doc *Document) { doc.Spec.WirelessScenarios[0].Station.Service = "another-deployment" }, "station"},
		{"fixed station", func(doc *Document) {
			doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, WirelessMedium{Name: "rf5", Band: "5ghz", Channel: 40, WidthMHz: 20})
			doc.Spec.Services[0].Radios[1].Medium = "rf5"
			doc.Spec.Services[1].Radios[0].Roaming = nil
			doc.Spec.Services[1].Radios[0].Medium = "rf24"
		}, "roaming station"},
		{"station replica", func(doc *Document) { doc.Spec.WirelessScenarios[0].Station.Replica = 2 }, "replica"},
		{"unknown AP", func(doc *Document) { doc.Spec.WirelessScenarios[0].APs[1].Service = "another-deployment" }, "AP"},
		{"wrong slot", func(doc *Document) { slot := 2; doc.Spec.WirelessScenarios[0].APs[1].Slot = &slot }, "slot"},
		{"missing slot", func(doc *Document) { doc.Spec.WirelessScenarios[0].APs[1].Slot = nil }, "slot"},
		{"duplicate AP", func(doc *Document) { doc.Spec.WirelessScenarios[0].APs[1] = doc.Spec.WirelessScenarios[0].APs[0] }, "duplicate AP"},
		{"outside roaming media", func(doc *Document) {
			doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, WirelessMedium{Name: "rf5", Band: "5ghz", Channel: 40, WidthMHz: 20})
			doc.Spec.Services[0].Radios[1].Medium = "rf5"
			third := doc.Spec.Services[0].Radios[0]
			third.Name, third.Device = "third-ap", "wlan2"
			doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, third)
		}, "eligible"},
		{"unknown final AP", func(doc *Document) { doc.Spec.WirelessScenarios[0].Assertions.FinalAP.Radio = "ghost" }, "finalAP"},
		{"same start and end", func(doc *Document) {
			doc.Spec.WirelessScenarios[0].Assertions.FinalAP = doc.Spec.WirelessScenarios[0].Assertions.InitialAP
		}, "different"},
		{"missing IPv4 assertion", func(doc *Document) { doc.Spec.WirelessScenarios[0].Assertions.SameIPv4 = false }, "sameIPv4"},
		{"negative time", func(doc *Document) { at := -1; doc.Spec.WirelessScenarios[0].Steps[1].AtMs = &at }, "atMs"},
		{"unordered time", func(doc *Document) { at := 0; doc.Spec.WirelessScenarios[0].Steps[2].AtMs = &at }, "order"},
		{"late time", func(doc *Document) { at := 120001; doc.Spec.WirelessScenarios[0].Steps[1].AtMs = &at }, "atMs"},
		{"invalid SNR", func(doc *Document) { snr := -101; doc.Spec.WirelessScenarios[0].Steps[1].SNRDb = &snr }, "snrDb"},
		{"missing SNR", func(doc *Document) { doc.Spec.WirelessScenarios[0].Steps[1].SNRDb = nil }, "snrDb"},
		{"duplicate link update", func(doc *Document) {
			doc.Spec.WirelessScenarios[0].Steps[1].AtMs = doc.Spec.WirelessScenarios[0].Steps[0].AtMs
		}, "duplicate"},
		{"unlisted step AP", func(doc *Document) { doc.Spec.WirelessScenarios[0].Steps[1].AP.Radio = "ghost" }, "step"},
		{"gap bound", func(doc *Document) { doc.Spec.WirelessScenarios[0].Assertions.MaxGapMs = 120001 }, "maxGapMs"},
		{"roam bound", func(doc *Document) { doc.Spec.WirelessScenarios[0].Assertions.MaxRoamMs = 0 }, "maxRoamMs"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := roamingScenarioDoc()
			test.edit(&doc)
			if err := Validate(doc); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadAndValidateMixedWirelessManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	content := strings.Join([]string{
		"apiVersion: vcpe.dev/v1", "kind: Deployment", "metadata: {name: edge}",
		"spec:", "  networks:", "    - role: wan", "      ipv4: {cidr: 10.7.200.0/24}",
		"  wirelessMedia:", "    - {name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}",
		"  wirelessNetworks:", "    - {name: home, ssid: vcpe-lab, security: open}",
		"  services:", "    - name: gateway", "      type: gateway", "      replicas: 1",
		"      image: {repository: gateway}", "      interfaces: [{role: wan}]",
		"      bridges: [{name: brlan0}]",
		"      radios: [{name: home-ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]",
		"    - name: station", "      type: generic-container", "      replicas: 1",
		"      image: {repository: wireless-client}",
		"      radios: [{name: home-station, medium: rf24, network: home, device: wlan0, mode: station, addressing: dhcp}]",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(doc.Spec.Services[0].Interfaces) != 1 || len(doc.Spec.Services[0].Radios) != 1 {
		t.Fatalf("mixed attachments were not decoded separately: %+v", doc.Spec.Services[0])
	}
}

func TestLoadAndValidateWirelessPersonalSecurityManifest(t *testing.T) {
	content := []byte(strings.Join([]string{
		"apiVersion: vcpe.dev/v1", "kind: Deployment", "metadata: {name: edge}",
		"spec:", "  networks: []", "  wirelessMedia:", "    - {name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}",
		"  wirelessNetworks:", "    - name: home", "      ssid: vcpe-lab",
		"      security: wpa3-personal", "      passphraseSecretRef: home-wifi",
		"  secrets:", "    - {name: home-wifi, provider: env, key: VCPE_HOME_WIFI_PASSPHRASE}",
		"  services:", "    - name: gateway", "      type: gateway", "      replicas: 1",
		"      image: {repository: gateway}", "      bridges: [{name: brlan0}]",
		"      radios: [{name: home-ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]",
	}, "\n"))
	doc, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	wireless := doc.Spec.WirelessNetworks[0]
	if wireless.Security != WirelessWPA3Personal || wireless.PassphraseSecretRef != "home-wifi" {
		t.Fatalf("wireless security declaration = %+v", wireless)
	}
}

func TestWirelessPersonalSecurityFixtures(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{name: "WPA2 personal", file: "wpa2-personal.yaml"},
		{name: "WPA3 personal", file: "wpa3-personal.yaml"},
		{name: "open with reference", file: "invalid-open-reference.yaml", wantErr: "cannot declare passphraseSecretRef"},
		{name: "unknown reference", file: "invalid-unknown-reference.yaml", wantErr: "unknown passphrase secret"},
		{name: "duplicate secret", file: "invalid-duplicate-secret.yaml", wantErr: "duplicate secret name"},
		{name: "inline secret", file: "invalid-inline-secret.yaml", wantErr: "field passphrase not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Load(filepath.Join("testdata", "wireless-security", test.file))
			if err == nil {
				err = Validate(doc)
			}
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("fixture %s: %v", test.file, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fixture %s error = %v, want %q", test.file, err, test.wantErr)
			}
		})
	}
}

func TestValidateAcceptsExistingManifestWithoutWirelessFields(t *testing.T) {
	doc := validDoc()
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidWirelessTopology(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{name: "duplicate profile", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks = append(doc.Spec.WirelessNetworks, doc.Spec.WirelessNetworks[0])
		}, want: "duplicate wireless network"},
		{name: "empty ssid", edit: func(doc *Document) { doc.Spec.WirelessNetworks[0].SSID = "" }, want: "SSID"},
		{name: "long ssid", edit: func(doc *Document) { doc.Spec.WirelessNetworks[0].SSID = strings.Repeat("x", 33) }, want: "SSID"},
		{name: "invalid channel", edit: func(doc *Document) { doc.Spec.WirelessMedia[0].Channel = 15 }, want: "channel"},
		{name: "unsupported security", edit: func(doc *Document) { doc.Spec.WirelessNetworks[0].Security = "wpa2" }, want: "security"},
		{name: "open with passphrase reference", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks[0].PassphraseSecretRef = "home-wifi"
		}, want: "cannot declare passphraseSecretRef"},
		{name: "personal without passphrase reference", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks[0].Security = WirelessWPA2Personal
		}, want: "requires passphraseSecretRef"},
		{name: "personal with unknown passphrase reference", edit: func(doc *Document) {
			doc.Spec.WirelessNetworks[0].Security = WirelessWPA3Personal
			doc.Spec.WirelessNetworks[0].PassphraseSecretRef = "missing"
		}, want: `unknown passphrase secret "missing"`},
		{name: "duplicate logical name", edit: func(doc *Document) {
			doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, doc.Spec.Services[0].Radios[0])
		}, want: "duplicate radio name"},
		{name: "duplicate device", edit: func(doc *Document) {
			radio := doc.Spec.Services[0].Radios[0]
			radio.Name = "other"
			doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, radio)
		}, want: "duplicate radio device"},
		{name: "invalid device", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].Device = "wireless/device-name" }, want: "invalid Linux device"},
		{name: "unknown profile", edit: func(doc *Document) { doc.Spec.Services[1].Radios[0].Network = "missing" }, want: "unknown wireless network"},
		{name: "unknown mode", edit: func(doc *Document) { doc.Spec.Services[1].Radios[0].Mode = "unknown" }, want: "invalid mode"},
		{name: "AP missing bridge", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Bridge = "" }, want: "undeclared bridge"},
		{name: "AP undeclared bridge", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].VAPs[0].Bridge = "brmissing" }, want: "undeclared bridge"},
		{name: "AP addressing", edit: func(doc *Document) { doc.Spec.Services[0].Radios[0].Addressing = AddressingDHCP }, want: "only in vaps"},
		{name: "station static", edit: func(doc *Document) { doc.Spec.Services[1].Radios[0].Addressing = AddressingStatic }, want: "must use dhcp addressing"},
		{name: "multiple APs", edit: func(doc *Document) {
			radio := doc.Spec.Services[0].Radios[0]
			radio.Name = "other"
			radio.Device = "wlan1"
			doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, radio)
		}, want: "at most one"},
		{name: "station without AP", edit: func(doc *Document) { doc.Spec.Services[0].Radios = nil }, want: "exactly one AP"},
		{name: "wired and wireless defaults", edit: func(doc *Document) {
			doc.Spec.Services[0].Radios = nil
			doc.Spec.Services[1].Interfaces = []Interface{{Role: "wan", DefaultRoute: true}}
			doc.Spec.Services[1].Radios[0].DefaultRoute = true
		}, want: "default routes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := wirelessDoc()
			tt.edit(&doc)
			err := Validate(doc)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsDuplicateSecretNames(t *testing.T) {
	doc := validDoc()
	doc.Spec.Secrets = []SecretRef{
		{Name: "home-wifi", Provider: "env", Key: "WIFI_ONE"},
		{Name: "home-wifi", Provider: "file", Key: "/run/secrets/wifi-two"},
	}
	err := Validate(doc)
	if err == nil || !strings.Contains(err.Error(), `duplicate secret name "home-wifi"`) {
		t.Fatalf("Validate() error = %v, want duplicate secret name", err)
	}
}

func TestValidateDevelopmentLiteralSecret(t *testing.T) {
	doc, err := Parse([]byte(strings.Join([]string{
		"apiVersion: vcpe.dev/v1", "kind: Deployment", "metadata:",
		"  name: edge", "  labels: {environment: development}",
		"spec:", "  networks: []", "  wirelessNetworks:",
		"    - {name: home, ssid: vcpe-lab, security: wpa3-personal, passphraseSecretRef: home-wifi}",
		"  secrets:", "    - {name: home-wifi, provider: literal, value: development-passphrase}",
		"  services:", "    - {name: client, type: generic-container, replicas: 1, image: {repository: client}}",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	delete(doc.Metadata.Labels, "environment")
	err = Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "requires metadata.labels.environment=development") {
		t.Fatalf("Validate() error = %v, want development-label requirement", err)
	}
}

func TestValidateSecretProviderFields(t *testing.T) {
	tests := []struct {
		name   string
		secret SecretRef
		want   string
	}{
		{name: "env requires key", secret: SecretRef{Name: "wifi", Provider: "env"}, want: "requires key and forbids value"},
		{name: "env forbids value", secret: SecretRef{Name: "wifi", Provider: "env", Key: "WIFI", Value: "not-allowed"}, want: "requires key and forbids value"},
		{name: "file requires key", secret: SecretRef{Name: "wifi", Provider: "file"}, want: "requires key and forbids value"},
		{name: "literal requires value", secret: SecretRef{Name: "wifi", Provider: "literal"}, want: "requires value and forbids key"},
		{name: "literal forbids key", secret: SecretRef{Name: "wifi", Provider: "literal", Key: "WIFI", Value: "development-passphrase"}, want: "requires value and forbids key"},
		{name: "unsupported", secret: SecretRef{Name: "wifi", Provider: "vault", Key: "WIFI"}, want: "unsupported secret provider"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := validDoc()
			doc.Metadata.Labels = map[string]string{"environment": "development"}
			doc.Spec.Secrets = []SecretRef{test.secret}
			err := Validate(doc)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

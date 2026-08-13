package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

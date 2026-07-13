package manifest

import (
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
						{Role: "wan", IPv4: "10.7.200.2", DefaultRoute: true},
						{Role: "lan", IPv4: "10.7.210.2"},
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
	doc.Spec.Services[0].Interfaces[1].IPv4 = ""
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

package contract_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
)

func TestContractBuildsAndValidatesRadioOnlyBinding(t *testing.T) {
	deployment := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{{
			Name: "home", SSID: "vcpe-lab", Channel: 1, Security: "open",
		}},
		Services: []plan.Service{{
			Name: "station",
			Instances: []plan.Instance{{
				Radios: []plan.Radio{{
					Name: "home-station", Network: "home", Device: "wlan0", Mode: "station",
					Addressing: "dhcp", DefaultRoute: true, MAC: "02:00:00:00:00:01",
				}},
			}},
		}},
	}
	document := contract.BuildForDeployment("op-radio", deployment)["station"]
	if err := contract.Validate(document); err != nil {
		t.Fatalf("validate radio-only contract: %v", err)
	}
	if len(document.Interfaces) != 0 || len(document.Radios) != 1 {
		t.Fatalf("bindings = interfaces:%d radios:%d", len(document.Interfaces), len(document.Radios))
	}
	radio := document.Radios[0]
	if radio.Name != "home-station" || radio.Device != "wlan0" || radio.SSID != "vcpe-lab" || radio.Channel != 1 || !radio.DefaultRoute {
		t.Fatalf("radio binding = %#v", radio)
	}
}

func TestContractAcceptsMeshAndRoamingBindingsWithoutSecretReference(t *testing.T) {
	mesh := plan.Radio{Name: "backhaul", Medium: "rf5", Device: "mesh0", MAC: "02:00:00:00:00:01", Mode: manifest.RadioModeMesh, Bridge: "brlan", Mesh: &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"}}
	roaming := plan.Radio{Name: "client", Network: "home", Device: "wlan0", MAC: "02:00:00:00:00:02", Mode: manifest.RadioModeStation, Addressing: "dhcp", RoamingMedia: []string{"rf5"}, Candidates: []plan.RoamingCandidate{
		{Service: "gateway", Replica: 1, Radio: "ap", Medium: "rf5", Band: "5ghz", Channel: 36, BSSID: "02:00:00:00:00:03"},
		{Service: "remote", Replica: 1, Radio: "ap", Medium: "rf5", Band: "5ghz", Channel: 36, BSSID: "02:00:00:00:00:04"},
	}}
	deployment := plan.Deployment{Name: "edge",
		WirelessMedia:    []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "wifi-key"}},
		Services: []plan.Service{
			{Name: "gateway", Instances: []plan.Instance{{Radios: []plan.Radio{mesh}}}},
			{Name: "station", Instances: []plan.Instance{{Radios: []plan.Radio{roaming}}}},
		},
	}
	for _, document := range contract.BuildForDeployment("op-radio", deployment) {
		if err := contract.Validate(document); err != nil {
			t.Fatalf("validate %s: %v", document.Service, err)
		}
		serialized, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serialized), "mesh-key") {
			t.Fatalf("mesh secret reference leaked into startup contract: %s", serialized)
		}
	}
	meshBinding := contract.BuildForDeployment("op-radio", deployment)["gateway"].Radios[0]
	if meshBinding.MeshID != "mesh-home" || meshBinding.MeshSAEFile != secrets.MeshCredentialContainerPath("mesh-home") {
		t.Fatalf("mesh binding = %+v", meshBinding)
	}
	roamingBinding := contract.BuildForDeployment("op-radio", deployment)["station"].Radios[0]
	if len(roamingBinding.Candidates) != 2 || roamingBinding.Candidates[1].BSSID != "02:00:00:00:00:04" {
		t.Fatalf("roaming candidates = %+v", roamingBinding.Candidates)
	}
	for _, test := range []struct {
		name    string
		service string
		change  func(*contract.RadioBinding)
	}{
		{"missing mesh file", "gateway", func(radio *contract.RadioBinding) { radio.MeshSAEFile = "" }},
		{"missing bridge", "gateway", func(radio *contract.RadioBinding) { radio.Bridge = "" }},
		{"roaming without candidates", "station", func(radio *contract.RadioBinding) { radio.Candidates = radio.Candidates[:1] }},
		{"roaming without credential", "station", func(radio *contract.RadioBinding) { radio.PassphraseFile = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := contract.BuildForDeployment("op-radio", deployment)[test.service]
			test.change(&document.Radios[0])
			if err := contract.Validate(document); err == nil {
				t.Fatal("invalid mesh or roaming startup contract was accepted")
			}
		})
	}
}

func TestContractPreservesWirelessPolicyWithoutCredentialBytes(t *testing.T) {
	const credential = "sentinel-passphrase-never-in-contract"
	deployment := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{{
			Name:                "home",
			SSID:                "vcpe-lab",
			Channel:             1,
			Security:            manifest.WirelessWPA3Personal,
			PassphraseSecretRef: "home-wifi",
		}},
		Services: []plan.Service{{
			Name: "station",
			Instances: []plan.Instance{{Radios: []plan.Radio{{
				Name: "home-station", Network: "home", Device: "wlan0", Mode: "station",
				Addressing: "dhcp", MAC: "02:00:00:00:00:01",
			}}}},
		}},
	}
	document := contract.BuildForDeployment("op-radio", deployment)["station"]
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	if !strings.Contains(serialized, `"security":"wpa3-personal"`) || !strings.Contains(serialized, `"passphraseSecretRef":"home-wifi"`) {
		t.Fatalf("contract omitted non-sensitive wireless policy: %s", serialized)
	}
	if strings.Contains(serialized, credential) {
		t.Fatalf("contract contains resolved credential bytes: %s", serialized)
	}
}

func TestContractBuildsMediaVAPAndStationBindings(t *testing.T) {
	deployment := plan.Deployment{
		Name:          "edge",
		WirelessMedia: []plan.WirelessMedium{{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "private-secret-ref"},
			{Name: "guest", SSID: "Guest", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "guest-secret-ref"},
		},
		Services: []plan.Service{
			{Name: "gateway", Instances: []plan.Instance{{Radios: []plan.Radio{{
				Name: "ap6", Medium: "rf6", Mode: manifest.RadioModeAP, Device: "wlan0", MAC: "02:00:00:00:00:01",
				VAPs: []plan.VAP{
					{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:00:01", Network: "home", Bridge: "brlan"},
					{Slot: 7, Device: "vap7", MAC: "02:00:00:00:00:07", Network: "guest", Bridge: "brguest"},
				},
			}}}}},
			{Name: "client", Instances: []plan.Instance{{Radios: []plan.Radio{{
				Name: "sta6", Medium: "rf6", Mode: manifest.RadioModeStation, Device: "wlan1", MAC: "02:00:00:00:00:02",
				Network: "home", APBSSID: "02:00:00:00:00:01", Addressing: manifest.AddressingDHCP,
			}}}}},
		},
	}
	contracts := contract.BuildForDeployment("op-media", deployment)
	for _, service := range []string{"gateway", "client"} {
		document := contracts[service]
		if err := contract.Validate(document); err != nil {
			t.Fatalf("%s startup contract: %v", service, err)
		}
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "private-secret-ref") || strings.Contains(string(data), "guest-secret-ref") ||
			!strings.Contains(string(data), secrets.WirelessCredentialContainerPath("home")) {
			t.Fatalf("%s contract leaked a secret reference or omitted protected path: %s", service, data)
		}
		radio := document.Radios[0]
		if radio.Medium != "rf6" || radio.Band != "6ghz" || radio.Channel != 5 || radio.WidthMHz != 20 {
			t.Fatalf("%s RF binding = %+v", service, radio)
		}
	}
	ap := contracts["gateway"]
	if contracts["client"].Radios[0].APBSSID != "02:00:00:00:00:01" {
		t.Fatalf("station target BSSID = %q", contracts["client"].Radios[0].APBSSID)
	}
	if len(ap.Radios[0].VAPs) != 2 || ap.Radios[0].VAPs[0].Slot != 0 || ap.Radios[0].VAPs[1].Slot != 7 ||
		ap.Radios[0].VAPs[1].Bridge != "brguest" {
		t.Fatalf("AP VAP binding = %+v", ap.Radios[0].VAPs)
	}
	for _, test := range []struct {
		name string
		edit func(*contract.Document)
		want string
	}{
		{"non-PSC channel", func(doc *contract.Document) { doc.Radios[0].Channel = 7 }, "PSC channel"},
		{"unsupported width", func(doc *contract.Document) { doc.Radios[0].WidthMHz = 40 }, "20 MHz"},
		{"open 6 GHz VAP", func(doc *contract.Document) { doc.Radios[0].VAPs[0].Security = manifest.WirelessOpen }, "WPA3-Personal"},
		{"missing credential", func(doc *contract.Document) { doc.Radios[0].VAPs[0].PassphraseFile = "" }, "credential file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := contracts["gateway"]
			document.Radios = append([]contract.RadioBinding(nil), document.Radios...)
			document.Radios[0].VAPs = append([]contract.VAPBinding(nil), document.Radios[0].VAPs...)
			test.edit(&document)
			if err := contract.Validate(document); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("contract validation = %v, want %q", err, test.want)
			}
		})
	}
	ap.Radios[0].VAPs[0], ap.Radios[0].VAPs[1] = ap.Radios[0].VAPs[1], ap.Radios[0].VAPs[0]
	if err := contract.Validate(ap); err == nil || !strings.Contains(err.Error(), "unordered VAP") {
		t.Fatalf("unordered VAP validation = %v", err)
	}
}

func TestContractWithoutRadiosOmitsRadioField(t *testing.T) {
	document := contract.Document{
		Version:    contract.SupportedVersion,
		Service:    "bng",
		Deployment: "edge",
		Interfaces: []contract.InterfaceBinding{{Role: "wan", Name: "eth0", MAC: "02:00:00:00:00:02"}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"radios"`) {
		t.Fatalf("no-radio contract unexpectedly contains radios: %s", data)
	}
	if err := contract.Validate(document); err != nil {
		t.Fatalf("validate no-radio contract: %v", err)
	}
}

// TestContractMatchesPlannerIdentities asserts that the runtime-init startup
// contract carries byte-for-byte the same interface identities (device, MAC,
// addresses, gateways) the planner resolved, and that those MACs match a fresh
// CanonicalMAC computation. This guards the invariant that the planner and the
// runtime-init contract never diverge.
func TestContractMatchesPlannerIdentities(t *testing.T) {
	doc := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{
				{Role: "wan", IPv4: &manifest.AddressFamily{CIDR: "10.200.0.0/24", Gateway: "10.200.0.1"}},
				{Role: "lan", IPv4: &manifest.AddressFamily{CIDR: "10.210.0.0/24", Gateway: "10.210.0.1"}},
			},
			Services: []manifest.Service{
				{
					Name:     "bng",
					Type:     "bng",
					Replicas: 1,
					Image:    manifest.Image{Repository: "ghcr.io/gdcs-dev/bng", Tag: "dev"},
					Interfaces: []manifest.Interface{
						{Role: "wan", IPv4: "10.200.0.2", DefaultRoute: true},
						{Role: "lan"},
					},
				},
			},
		},
	}

	resolved, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatalf("planner build: %v", err)
	}

	contracts := contract.BuildForDeployment("op-test", resolved)
	doc0, ok := contracts["bng"]
	if !ok {
		t.Fatal("expected a bng contract")
	}
	if doc0.Deployment != "edge" {
		t.Fatalf("expected deployment edge, got %q", doc0.Deployment)
	}

	plannedIface := resolved.Services[0].Instances[0].Interfaces
	if len(doc0.Interfaces) != len(plannedIface) {
		t.Fatalf("interface count mismatch: contract %d, planner %d", len(doc0.Interfaces), len(plannedIface))
	}

	for i, binding := range doc0.Interfaces {
		want := plannedIface[i]
		if binding.Role != want.Role {
			t.Errorf("iface %d role: contract %q, planner %q", i, binding.Role, want.Role)
		}
		if binding.Name != want.Device {
			t.Errorf("iface %d device: contract %q, planner %q", i, binding.Name, want.Device)
		}
		if binding.MAC != want.MAC {
			t.Errorf("iface %d mac: contract %q, planner %q", i, binding.MAC, want.MAC)
		}
		// The MAC must equal a fresh CanonicalMAC for this single-replica key.
		expectMAC := plan.CanonicalMAC("edge", "bng", want.Role, 0)
		if binding.MAC != expectMAC {
			t.Errorf("iface %d mac %q does not match CanonicalMAC %q", i, binding.MAC, expectMAC)
		}
		if binding.Gateway4 != want.Gateway4 {
			t.Errorf("iface %d gateway4: contract %q, planner %q", i, binding.Gateway4, want.Gateway4)
		}
	}
}

// TestBridgeNameDeterminismMatchesPlanner asserts the planner derives the same
// bridge name DeriveBridgeName produces for the same (deployment, role) key.
func TestBridgeNameDeterminismMatchesPlanner(t *testing.T) {
	doc := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{
				{Role: "wan", IPv4: &manifest.AddressFamily{CIDR: "10.200.0.0/24"}},
			},
			Services: []manifest.Service{
				{Name: "bng", Type: "bng", Replicas: 1, Image: manifest.Image{Repository: "x/bng"}, Interfaces: []manifest.Interface{{Role: "wan"}}},
			},
		},
	}
	resolved, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatalf("planner build: %v", err)
	}
	want, _ := plan.DeriveBridgeName("edge", "wan")
	if got := resolved.Networks[0].Bridge; got != want {
		t.Fatalf("bridge name mismatch: planner %q, DeriveBridgeName %q", got, want)
	}
}

package planner

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

func TestMeshRoamingBothGatewaysHaveDistinctManagedWANs(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(doc); err != nil {
		t.Fatal(err)
	}
	resolved, err := Build(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Networks) != 2 || len(resolved.Services) != 6 {
		t.Fatalf("expected management and WAN networks with BNG, WebPA and two Gateways: %+v", resolved)
	}
	var rootMAC, remoteMAC string
	for _, service := range resolved.Services {
		if service.Name != "root" && service.Name != "remote" {
			continue
		}
		interfaces := service.Instances[0].Interfaces
		if len(interfaces) != 1 || interfaces[0].Role != "wan" || interfaces[0].Device != "erouter0" || interfaces[0].MAC == "" {
			t.Fatalf("%s must have a single managed WAN, off the client bridge: %+v", service.Name, interfaces)
		}
		if service.Name == "root" {
			rootMAC = interfaces[0].MAC
		} else {
			remoteMAC = interfaces[0].MAC
		}
	}
	if rootMAC == "" || remoteMAC == "" || rootMAC == remoteMAC {
		t.Fatalf("Gateways must have distinct WAN device identities: root=%q remote=%q", rootMAC, remoteMAC)
	}
}

func TestTwoGatewayMeshRoamingExample(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(doc); err != nil {
		t.Fatal(err)
	}
	resolved, err := Build(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Networks) != 2 || len(resolved.Services) != 6 || len(resolved.WirelessScenarios) != 1 {
		t.Fatalf("unexpected topology: %+v", resolved)
	}
	scenario := resolved.WirelessScenarios[0]
	if len(scenario.APs) != 2 || len(scenario.Steps) != 4 || scenario.APs[0].BSSID == scenario.APs[1].BSSID ||
		scenario.Assertions.InitialAP.BSSID != scenario.APs[0].BSSID || scenario.Assertions.FinalAP.BSSID != scenario.APs[1].BSSID {
		t.Fatalf("scenario did not resolve both distinct APs: %+v", scenario)
	}
	var probe plan.Service
	for _, service := range resolved.Services {
		switch service.Name {
		case "root", "remote":
			if len(service.Instances[0].Interfaces) != 1 || service.Instances[0].Interfaces[0].Role != "wan" || len(service.Instances[0].Radios) != 2 || service.Instances[0].Radios[0].Mesh == nil {
				t.Fatalf("Gateway has a wired bypass or missing mesh radio: %+v", service)
			}
		case "remote-probe":
			probe = service
		}
	}
	if len(probe.Instances) != 1 || len(probe.Instances[0].Interfaces) != 0 || len(probe.Instances[0].Radios) != 1 ||
		probe.Instances[0].Radios[0].APBSSID != scenario.Assertions.FinalAP.BSSID {
		t.Fatalf("remote DHCP probe must be fixed to the remote AP without a wired path: %+v", probe)
	}
}

func TestRootAloneServesClientLAN(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(script), "\ncheck_remote_path() {")
	end := strings.Index(string(script)[start+1:], "\n}\n")
	if start < 0 || end < 0 {
		t.Fatal("remote-path check missing from mesh smoke")
	}
	check := string(script)[start+1 : start+1+end+2]
	stub := `podman() {
    case "$*" in
        *"gateway-health-probe mesh"*) printf '%s\n' '{"radio":"backhaul","interface":"mesh0","bridge":"brlan0","joined":true,"peers":1}' ;;
        *"grep -Fxq"*|*"grep -Eq"*|*"ping -n"*) return 0 ;;
        *"cat /run/vcpe/wireless-client/status"*) printf '%s\n' 'state=ready' 'ipv4=10.0.0.120/24' ;;
        *"cat /sys/class/net/wlan0/address"*) printf '%s\n' '02:00:00:00:00:12' ;;
        *"cat /var/lib/misc/dnsmasq.leases"*) printf '%s\n' "$ROOT_LEASE" ;;
        *"ip -j -4 addr show dev brlan0"*) printf '%s\n' "$REMOTE_ADDRESS" ;;
        *"pgrep -x dnsmasq"*) [[ "$REMOTE_DHCP" == 1 ]] ;;
    esac
}
sleep() { :; }
`
	run := func(lease, address, dhcp string) error {
		command := exec.Command("bash", "-c", "set -euo pipefail\n"+stub+check+"\ncheck_remote_path mesh-roaming")
		command.Env = append(os.Environ(), "ROOT_LEASE="+lease, "REMOTE_ADDRESS="+address, "REMOTE_DHCP="+dhcp)
		output, err := command.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "remote mesh DHCP/LAN path did not recover") {
			t.Fatalf("unexpected smoke check failure: %s: %v", output, err)
		}
		return err
	}
	lease := "9999999999 02:00:00:00:00:12 10.0.0.120 remote-probe *"
	noAddress := `[{"addr_info":[]}]`
	if err := run(lease, noAddress, "0"); err != nil {
		t.Fatalf("root-owned lease and remote AP path rejected: %v", err)
	}
	if err := run("", noAddress, "0"); err == nil {
		t.Fatal("remote client accepted without a matching root DHCP lease")
	}
	if err := run(lease, `[{"addr_info":[{"family":"inet","local":"10.0.0.2"}]}]`, "0"); err == nil {
		t.Fatal("remote bridge accepted with a competing client-LAN address")
	}
	if err := run(lease, noAddress, "1"); err == nil {
		t.Fatal("remote Gateway accepted with a competing DHCP server")
	}
}

func TestMeshSmokeChecksBothGatewaysBeforeAndDuringMeshLoss(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	first := strings.Index(source, "check_management \"$primary_manifest\"")
	last := strings.LastIndex(source, "check_management \"$primary_manifest\"")
	disruption := strings.Index(source, "mesh_disabled=true")
	repair := strings.Index(source, "echo 'mesh smoke: restore remote mesh service after path repair'")
	if !strings.Contains(source, "go -C \"$repo_root/controlplane\" run ./tests/meshwebpa") ||
		first < 0 || first == last || disruption < 0 || repair < 0 || first > disruption || last < disruption || last > repair {
		t.Fatal("live smoke must check both WebPA management paths before and during primary mesh loss")
	}
}

func TestMeshSmokeUsesLocalGatewayBuild(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range doc.Spec.Services {
		if service.Type == "gateway" && (service.Image.Repository != "ghcr.io/gdcs-dev/gateway" || service.Image.Tag != "dev" || service.Image.PullPolicy != "build-if-missing") {
			t.Fatalf("%s must use the locally built development Gateway: %+v", service.Name, service.Image)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "VCPE_MESH_GATEWAY_DIGEST") || strings.Contains(string(script), `image["repository"] =`) || strings.Contains(string(script), `image["pullPolicy"] = "always-pull"`) {
		t.Fatal("mesh smoke must preserve the local Gateway build rather than require a published digest")
	}
}

func TestMeshSmokeKeepsDevelopmentWirelessKeys(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mesh-key", "wlan-key"} {
		found := false
		for _, secret := range doc.Spec.Secrets {
			if secret.Name == name {
				found = secret.Provider == "literal" && len(secret.Value) >= 8 && len(secret.Value) <= 63
			}
		}
		if !found {
			t.Fatalf("%s must have a usable development-only literal", name)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "VCPE_MESH_KEY") || strings.Contains(string(script), "VCPE_WLAN_KEY") {
		t.Fatal("mesh smoke must use the manifest's development keys without extra environment setup")
	}
}

func TestMeshSmokeRestoresPodmanWANBeforeRemoteRecreation(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	if !strings.Contains(source, "podman exec \"$remote\" ip link set erouter0 name eth1\n    podman restart \"$remote\"") ||
		strings.Count(source, "restart_remote_gateway\n") != 1 {
		t.Fatal("remote Gateway recreation must restore the WAN name expected by Podman")
	}
}

func TestMeshSmokeRepairsMeshWithoutRecreatingRemoteRadio(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	start := strings.Index(source, "echo 'mesh smoke: restore remote mesh service after path repair'")
	end := strings.Index(source, "echo 'mesh smoke: run automatic roaming scenario'")
	if start < 0 || end <= start {
		t.Fatal("mesh repair or roaming phase missing")
	}
	repair := source[start:end]
	if !strings.Contains(repair, "podman exec \"$remote\" systemctl restart vcpe-mesh") ||
		strings.Contains(repair, "restart_remote_gateway\n") || !strings.Contains(repair, "check_remote_path \"$primary\"") {
		t.Fatal("mesh repair must restore the remote path without recreating its managed radio")
	}
}

func TestMeshSmokeWaitsForStationPathAfterRepair(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "smoke", "controlplane-mesh-roaming-machine.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	start := strings.Index(source, "\ncheck_station_path() {")
	if start < 0 {
		t.Fatal("station path check missing")
	}
	end := strings.Index(source[start+1:], "\n}\n")
	if end < 0 {
		t.Fatal("station path check has no end")
	}
	check := source[start+1 : start+1+end+2]
	command := exec.Command("bash", "-c", `set -euo pipefail
primary=mesh-roaming
attempts=0
podman() { attempts=$((attempts + 1)); [[ "$attempts" -ge 2 ]]; }
sleep() { :; }
`+check+`
check_station_path 'remote Gateway repair'
printf 'attempts=%s\n' "$attempts"
`)
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "attempts=2") {
		t.Fatalf("station recovery did not retry the transient first failure: %s: %v", output, err)
	}
}

func TestResolveMediaRadioAndOrderedVAPs(t *testing.T) {
	media := resolveWirelessMedia([]manifest.WirelessMedium{{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20}})
	if len(media) != 1 || media[0] != (plan.WirelessMedium{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20}) {
		t.Fatalf("resolved wireless medium = %+v", media)
	}
	service := manifest.Service{
		Name: "gateway", Replicas: 1,
		Radios: []manifest.Radio{{
			Name: "ap6", Device: "wlan0", Mode: manifest.RadioModeAP, Medium: "rf6",
			VAPs: []manifest.VAP{
				{Slot: 4, Network: "guest", Bridge: "brguest"},
				{Slot: 0, Network: "home", Bridge: "brlan"},
				{Slot: 1, Network: "iot", Bridge: "briot"},
			},
		}},
	}
	instance := resolveInstance("edge", service, 0, 1, nil)
	if len(instance.Interfaces) != 0 || len(instance.Radios) != 1 {
		t.Fatalf("wireless media created Podman attachments or lost radio: %+v", instance)
	}
	radio := instance.Radios[0]
	if radio.Medium != "rf6" || radio.Network != "" || radio.Bridge != "" || len(radio.VAPs) != 3 {
		t.Fatalf("resolved AP radio = %+v", radio)
	}
	for position, slot := range []int{0, 1, 4} {
		vap := radio.VAPs[position]
		if vap.Slot != slot || vap.MAC != plan.CanonicalVAPBSSID("edge", "gateway", 0, "ap6", slot) {
			t.Fatalf("resolved VAP in position %d = %+v", position, vap)
		}
	}
	if radio.VAPs[0].Device != "wlan0" || radio.VAPs[0].MAC != radio.MAC || radio.VAPs[1].Device != plan.VAPDeviceName("edge", "gateway", 0, "ap6", 1) {
		t.Fatalf("slot 0 must use primary identity, secondary VAP must have own identity: %+v", radio.VAPs)
	}
	if radio.VAPs[2].Network != "guest" || radio.VAPs[2].Bridge != "brguest" {
		t.Fatalf("VAP profile/bridge did not follow slot: %+v", radio.VAPs[2])
	}
	service.Radios = []manifest.Radio{{Name: "sta6", Device: "wlan1", Mode: manifest.RadioModeStation, Medium: "rf6", Network: "home"}}
	station := resolveInstance("edge", service, 0, 1, nil).Radios[0]
	if station.Medium != "rf6" || station.Network != "home" || len(station.VAPs) != 0 || station.Addressing != manifest.AddressingDHCP {
		t.Fatalf("station did not bind to explicit medium/profile with DHCP default: %+v", station)
	}
}

func TestBuildResolvedKeepsMediaOffPodmanNetworks(t *testing.T) {
	doc := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			WirelessMedia:    []manifest.WirelessMedium{{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20}},
			WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal}},
			Services: []manifest.Service{
				{Name: "gateway", Replicas: 1, Radios: []manifest.Radio{{
					Name: "ap6", Mode: manifest.RadioModeAP, Device: "wlan0", Medium: "rf6",
					VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}},
				}}},
				{Name: "client", Replicas: 1, DependsOn: []string{"gateway"}, Radios: []manifest.Radio{{
					Name: "sta6", Mode: manifest.RadioModeStation, Device: "wlan0", Medium: "rf6", Network: "home",
				}}},
			},
		},
	}
	resolved, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Networks) != 0 || len(resolved.WirelessMedia) != 1 || len(resolved.WirelessNetworks) != 1 ||
		resolved.WirelessMedium("rf6").Channel != 5 || resolved.WirelessNetwork("home").SSID != "Home" {
		t.Fatalf("unexpected media/profile or Podman network plan: %+v", resolved)
	}
	if len(resolved.Services) != 2 || len(resolved.Services[0].Instances[0].Radios[0].VAPs) != 1 ||
		resolved.Services[1].Instances[0].Radios[0].Medium != "rf6" ||
		resolved.Services[1].Instances[0].Radios[0].APBSSID != resolved.Services[0].Instances[0].Radios[0].VAPs[0].MAC {
		t.Fatalf("media AP/station bindings missing from plan: %+v", resolved.Services)
	}
	if public, err := Build(doc, nil); err != nil || !reflect.DeepEqual(public, resolved) {
		t.Fatalf("public planner must resolve media/VAP deployment: %v %+v", err, public)
	}
	doc.Spec.Services[0].Replicas = 2
	scaled, err := buildResolved(doc, nil)
	if err != nil || scaled.Services[1].Instances[0].Radios[0].APBSSID != resolved.Services[0].Instances[0].Radios[0].VAPs[0].MAC {
		t.Fatalf("station must remain on first AP replica after scaling: %v %+v", err, scaled.Services)
	}
	doc.Spec.Services[1].Radios[0].Network = "missing"
	if _, err := buildResolved(doc, nil); err == nil || !strings.Contains(err.Error(), "has no AP") {
		t.Fatalf("unmatched station must fail: %v", err)
	}
	doc.Spec.Services[1].Radios[0].Network = "home"
	doc.Spec.Services = append(doc.Spec.Services, manifest.Service{
		Name: "other-gateway", Replicas: 1, Radios: []manifest.Radio{{
			Name: "other-ap", Mode: manifest.RadioModeAP, Device: "wlan2", Medium: "rf6",
			VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}},
		}},
	})
	if _, err := buildResolved(doc, nil); err == nil || !strings.Contains(err.Error(), "multiple AP radios") {
		t.Fatalf("duplicate AP declarations must fail: %v", err)
	}
}

func TestBuildResolvesRoamingCandidatesWithoutPinningBSSID(t *testing.T) {
	doc := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			WirelessMedia:    []manifest.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
			WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", SSID: "home", Security: manifest.WirelessOpen}},
			Services: []manifest.Service{
				{Name: "gateway-b", Replicas: 1, Radios: []manifest.Radio{{Name: "ap", Mode: manifest.RadioModeAP, Device: "wlan0", Medium: "rf5", VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}}}}},
				{Name: "gateway-a", Replicas: 1, Radios: []manifest.Radio{{Name: "ap", Mode: manifest.RadioModeAP, Device: "wlan0", Medium: "rf5", VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}}}}},
				{Name: "client", Replicas: 1, Radios: []manifest.Radio{{Name: "sta", Mode: manifest.RadioModeStation, Device: "wlan0", Network: "home", Roaming: &manifest.Roaming{Media: []string{"rf5"}}}}},
			},
		},
	}
	resolved, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	var station plan.Radio
	for _, service := range resolved.Services {
		if service.Name == "client" {
			station = service.Instances[0].Radios[0]
		}
	}
	if station.APBSSID != "" || station.Medium != "" || len(station.Candidates) != 2 {
		t.Fatalf("roaming station should have two unpinned candidates: %+v", station)
	}
	for index, service := range []string{"gateway-a", "gateway-b"} {
		candidate := station.Candidates[index]
		if candidate.Service != service || candidate.Medium != "rf5" || candidate.Slot != 0 || candidate.BSSID != plan.CanonicalVAPBSSID("edge", service, 0, "ap", 0) {
			t.Fatalf("candidate %d = %+v", index, candidate)
		}
	}
	doc.Spec.WirelessMedia = append(doc.Spec.WirelessMedia, manifest.WirelessMedium{Name: "rf40", Band: "5ghz", Channel: 40, WidthMHz: 20})
	doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, manifest.Radio{
		Name: "other", Mode: manifest.RadioModeAP, Device: "wlan1", Medium: "rf40",
		VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}},
	})
	doc.Spec.Services[2].Radios[0].Roaming.Media = []string{"rf5", "rf40"}
	multichannel, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range multichannel.Services {
		if service.Name == "client" {
			candidates := service.Instances[0].Radios[0].Candidates
			if len(candidates) != 3 || candidates[2].Medium != "rf40" || candidates[2].Channel != 40 {
				t.Fatalf("cross-channel candidates = %+v", candidates)
			}
		}
	}
	doc.Spec.Services[0].Radios = doc.Spec.Services[0].Radios[:1]
	doc.Spec.Services[2].Radios[0].Roaming = nil
	doc.Spec.Services[2].Radios[0].Medium = "rf5"
	if _, err := buildResolved(doc, nil); err == nil || !strings.Contains(err.Error(), "multiple AP radios") {
		t.Fatalf("fixed station must reject ambiguous AP pair: %v", err)
	}
}

func TestBuildResolvesScenarioToPlannedCandidates(t *testing.T) {
	firstSlot, secondSlot := 0, 0
	first := manifest.VAPReference{Service: "gateway-a", Replica: 1, Radio: "ap", Slot: &firstSlot}
	second := manifest.VAPReference{Service: "gateway-b", Replica: 1, Radio: "ap", Slot: &secondSlot}
	atStart, atEnd, strong, weak := 0, 1000, 35, -75
	doc := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			WirelessMedia:    []manifest.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
			WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", SSID: "home", Security: manifest.WirelessOpen}},
			Services: []manifest.Service{
				{Name: "gateway-b", Replicas: 1, Radios: []manifest.Radio{{Name: "ap", Mode: manifest.RadioModeAP, Device: "wlan0", Medium: "rf5", VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}}}}},
				{Name: "gateway-a", Replicas: 1, Radios: []manifest.Radio{{Name: "ap", Mode: manifest.RadioModeAP, Device: "wlan0", Medium: "rf5", VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}}}}},
				{Name: "client", Replicas: 1, Radios: []manifest.Radio{{Name: "sta", Mode: manifest.RadioModeStation, Device: "wlan0", Network: "home", Roaming: &manifest.Roaming{Media: []string{"rf5"}}}}},
			},
			WirelessScenarios: []manifest.WirelessScenario{{
				Name: "crossover", Station: manifest.RadioReference{Service: "client", Replica: 1, Radio: "sta"}, APs: []manifest.VAPReference{first, second},
				Steps:      []manifest.RFStep{{AtMs: &atStart, AP: first, SNRDb: &strong}, {AtMs: &atEnd, AP: first, SNRDb: &weak}, {AtMs: &atEnd, AP: second, SNRDb: &strong}},
				Assertions: manifest.RoamAssertions{InitialAP: first, FinalAP: second, SameIPv4: true, MaxRoamMs: 5000, MaxGapMs: 3000},
			}},
		},
	}
	resolved, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.WirelessScenarios) != 1 {
		t.Fatalf("scenarios = %+v", resolved.WirelessScenarios)
	}
	scenario := resolved.WirelessScenarios[0]
	if scenario.Name != "crossover" || scenario.Station.Service != "client" || scenario.Station.Replica != 0 || len(scenario.APs) != 2 || len(scenario.Steps) != 3 {
		t.Fatalf("scenario = %+v", scenario)
	}
	if scenario.Assertions.InitialAP.BSSID != plan.CanonicalVAPBSSID("edge", "gateway-a", 0, "ap", 0) || scenario.Assertions.FinalAP.BSSID != plan.CanonicalVAPBSSID("edge", "gateway-b", 0, "ap", 0) || scenario.Steps[2].AP.BSSID != scenario.Assertions.FinalAP.BSSID {
		t.Fatalf("unresolved AP identities: %+v", scenario)
	}
}

func TestBuildCarriesMeshBridgeIntoRadioOwnership(t *testing.T) {
	doc := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{Services: []manifest.Service{{
			Name: "gateway", Replicas: 1, Radios: []manifest.Radio{{
				Name: "backhaul", Mode: manifest.RadioModeMesh, Medium: "rf5", Device: "mesh0",
				Mesh: &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"},
			}},
		}}},
	}
	resolved, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	radio := resolved.Services[0].Instances[0].Radios[0]
	if radio.Bridge != "brlan" || radio.Mesh == nil || radio.Mesh.Bridge != radio.Bridge {
		t.Fatalf("mesh bridge lost from planned radio ownership: %+v", radio)
	}
}

func TestWirelessIdentityCollisions(t *testing.T) {
	base := func() plan.Deployment {
		return plan.Deployment{Services: []plan.Service{{
			Name: "gateway", Instances: []plan.Instance{{
				InstanceName: "gateway-1",
				Interfaces:   []plan.Interface{{Role: "lan", Device: "eth0", MAC: "02:00:00:00:00:01"}},
				Radios: []plan.Radio{{
					Name: "ap", Device: "wlan0", MAC: "02:00:00:00:00:02",
					VAPs: []plan.VAP{
						{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:00:02"},
						{Slot: 1, Device: "vap1", MAC: "02:00:00:00:00:03"},
					},
				}},
			}},
		}}}
	}
	if err := checkWirelessIdentityCollisions(base()); err != nil {
		t.Fatalf("distinct wired, primary, and VAP identities: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*plan.Deployment)
		want string
	}{
		{"wired and primary MAC", func(dep *plan.Deployment) { dep.Services[0].Instances[0].Radios[0].MAC = "02:00:00:00:00:01" }, "MAC"},
		{"wired and secondary MAC", func(dep *plan.Deployment) { dep.Services[0].Instances[0].Radios[0].VAPs[1].MAC = "02:00:00:00:00:01" }, "MAC"},
		{"secondary and primary device", func(dep *plan.Deployment) { dep.Services[0].Instances[0].Radios[0].VAPs[1].Device = "wlan0" }, "interface name"},
		{"wired and secondary device", func(dep *plan.Deployment) { dep.Services[0].Instances[0].Radios[0].VAPs[1].Device = "eth0" }, "interface name"},
		{"slot zero mismatch", func(dep *plan.Deployment) { dep.Services[0].Instances[0].Radios[0].VAPs[0].MAC = "02:00:00:00:00:04" }, "slot 0 identity"},
		{"cross-service MAC", func(dep *plan.Deployment) {
			dep.Services = append(dep.Services, plan.Service{Name: "client", Instances: []plan.Instance{{
				InstanceName: "client-1", Radios: []plan.Radio{{Name: "station", Device: "wlan0", MAC: "02:00:00:00:00:03"}},
			}}})
		}, "MAC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dep := base()
			test.edit(&dep)
			err := checkWirelessIdentityCollisions(dep)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "gateway-1") {
				t.Fatalf("collision error = %v, want %q and owner", err, test.want)
			}
		})
	}
}

func TestTriBandVAPIdentitySurvivesProfileBridgeAndReplicaChanges(t *testing.T) {
	doc := manifest.Document{Metadata: manifest.Metadata{Name: "edge"}}
	doc.Spec.WirelessMedia = []manifest.WirelessMedium{
		{Name: "rf24", Band: "2.4ghz", Channel: 1, WidthMHz: 20},
		{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20},
		{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20},
	}
	doc.Spec.Services = []manifest.Service{{Name: "gateway", Replicas: 1}}
	for _, medium := range doc.Spec.WirelessMedia {
		doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, manifest.Radio{
			Name: medium.Name, Medium: medium.Name, Device: "wlan" + medium.Name, Mode: manifest.RadioModeAP,
			VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}, {Slot: 7, Network: "guest", Bridge: "brguest"}},
		})
	}
	first, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := buildResolved(doc, nil)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("repeated planning changed identities: %v", err)
	}
	doc.Spec.Services[0].Replicas = 2
	doc.Spec.Services[0].Radios[0].VAPs[1].Network = "rotated"
	doc.Spec.Services[0].Radios[0].VAPs[1].Bridge = "brnew"
	doc.Spec.Services[0].Radios[1].VAPs = doc.Spec.Services[0].Radios[1].VAPs[:1]
	scaled, err := buildResolved(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := first.Services[0].Instances[0].Radios
	after := scaled.Services[0].Instances[0].Radios
	if len(before) != 3 || len(after) != 3 || len(scaled.Services[0].Instances) != 2 {
		t.Fatalf("unexpected tri-band replica plan: %+v", scaled.Services)
	}
	for position := range before {
		if before[position].ManagerName != after[position].ManagerName || before[position].MAC != after[position].MAC ||
			before[position].VAPs[0].Device != after[position].VAPs[0].Device || before[position].VAPs[0].MAC != after[position].VAPs[0].MAC {
			t.Fatalf("primary identity changed for radio %s", before[position].Name)
		}
	}
	if before[0].VAPs[1].Device != after[0].VAPs[1].Device || before[0].VAPs[1].MAC != after[0].VAPs[1].MAC ||
		after[0].VAPs[1].Network != "rotated" || after[0].VAPs[1].Bridge != "brnew" {
		t.Fatal("profile/bridge reassignment changed slot 7 identity or was not resolved")
	}
	if before[0].ManagerName == scaled.Services[0].Instances[1].Radios[0].ManagerName ||
		before[0].VAPs[1].MAC == scaled.Services[0].Instances[1].Radios[0].VAPs[1].MAC {
		t.Fatal("new replica reused manager or VAP identity")
	}
}

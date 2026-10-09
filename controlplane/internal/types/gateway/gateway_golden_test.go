package gateway_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
	"gopkg.in/yaml.v3"
)

func TestGATEWAYGoldenComposeEnv(t *testing.T) {
	gateway.Register()
	st, ok := typeregistry.Lookup("gateway")
	if !ok {
		t.Fatal("gateway not registered")
	}

	var cfg yaml.Node
	if err := yaml.Unmarshal([]byte("lan: { ipv4: 192.168.0.1, ipv6: \"fd00::1\" }\nerouter: { vlan: 100 }\n"), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "lan", Bridge: "edge-lan"}}}
	svc := plan.Service{
		Name:   "gateway",
		Type:   "gateway",
		Image:  manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:01"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	got := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			got = a.Content
		}
	}
	for _, frag := range []string{
		"DEPLOYMENT_NAME=edge",
		"SERVICE_NAME=gateway",
		"IFACE_LAN_DEVICE=eth0",
		"LAN_IPV4=192.168.0.1",
		"LAN_IPV6=fd00::1",
		"EROUTER_VLAN=100",
	} {
		if !strings.Contains(got, frag) {
			t.Fatalf("gateway compose.env missing %q in:\n%s", frag, got)
		}
	}
}

func TestGatewayMeshLANUsesBridgeOnly(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	for _, test := range []struct {
		name   string
		bridge manifest.BridgeSpec
	}{
		{"root", manifest.BridgeSpec{Name: "brlan0", IPv4: "10.0.0.1/24", DHCPStart: "10.0.0.100", DHCPEnd: "10.0.0.200"}},
		{"remote", manifest.BridgeSpec{Name: "brlan0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := plan.Service{
				Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
				Bridges: []manifest.BridgeSpec{test.bridge},
				Instances: []plan.Instance{{Radios: []plan.Radio{{Mode: manifest.RadioModeMesh,
					Medium: "rf5", Mesh: &manifest.Mesh{ID: "lab-mesh", Bridge: "brlan0", SAESecretRef: "mesh-key"}}}}},
			}
			result, err := serviceType.Renderer().Render(context.Background(), render.Input{
				Deployment: plan.Deployment{Name: "edge", WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36}}}, Service: service,
			})
			if err != nil {
				t.Fatal(err)
			}
			var env string
			for _, artifact := range result.Artifacts {
				if artifact.Key == "compose.env" {
					env = artifact.Content
				}
			}
			for _, field := range []string{"BRIDGE_BRLAN0_IPV4=" + test.bridge.IPv4, "BRIDGE_BRLAN0_DHCP_START=" + test.bridge.DHCPStart, "LAN_BRIDGE=brlan0"} {
				if !strings.Contains(env, field) {
					t.Fatalf("missing %q in:\n%s", field, env)
				}
			}
			for _, legacy := range []string{"LAN_IPV4=", "BRLAN0_IPV4=", "BRLAN0_DHCP_START=", "BRLAN0_DHCP_END="} {
				if strings.Contains("\n"+env, "\n"+legacy) {
					t.Fatalf("mesh %s emits legacy fallback %q:\n%s", test.name, legacy, env)
				}
			}
		})
	}
}

func TestGatewayMeshRendersProtectedStartupPolicy(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	mesh := &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"}
	service := plan.Service{Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
		Bridges:   []manifest.BridgeSpec{{Name: "brlan"}},
		Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "backhaul", Device: "mesh0", Mode: manifest.RadioModeMesh, Medium: "rf5", Mesh: mesh}}}}}
	handle := render.SecretFileHandle{HostPath: "/private/mesh", ContainerPath: secrets.MeshCredentialContainerPath(mesh.ID)}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge", WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36}}}, Service: service,
		WirelessCredentialFiles: map[string]render.SecretFileHandle{"mesh:" + mesh.ID: handle}})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	for _, field := range []string{"device=mesh0", "id=mesh-home", "bridge=brlan", "frequency=5180", "credential=" + handle.ContainerPath} {
		if !strings.Contains(artifacts["mesh/backhaul.conf"], field+"\n") {
			t.Fatalf("mesh policy missing %q: %v", field, artifacts)
		}
	}
	for _, field := range []string{"./mesh:/etc/vcpe/mesh:ro", handle.HostPath + ":" + handle.ContainerPath + ":ro"} {
		if !strings.Contains(artifacts["compose.yaml"], field) {
			t.Fatalf("compose missing %q: %v", field, artifacts)
		}
	}
	bridgeMAC := plan.CanonicalMAC("edge", "gateway", "bridge/brlan", 0)
	if !strings.Contains(artifacts["compose.env"], "BRIDGE_BRLAN_MAC="+bridgeMAC) {
		t.Fatalf("Gateway bridge MAC missing from compose.env")
	}
	service.Name = "remote"
	remote, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge", WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36}}}, Service: service,
		WirelessCredentialFiles: map[string]render.SecretFileHandle{"mesh:" + mesh.ID: handle}})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range remote.Artifacts {
		if artifact.Key == "compose.env" && (!strings.Contains(artifact.Content, "BRIDGE_BRLAN_MAC="+plan.CanonicalMAC("edge", "remote", "bridge/brlan", 0)) || strings.Contains(artifact.Content, "BRIDGE_BRLAN_MAC="+bridgeMAC)) {
			t.Fatal("mesh Gateways must use distinct bridge MACs")
		}
	}
	for _, artifact := range artifacts {
		if strings.Contains(artifact, "mesh-key") {
			t.Fatal("mesh credential reference leaked into rendered artifact")
		}
	}
}

func TestGATEWAYRejectsUnknownConfigField(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("bogus: true\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestGatewayRendersOpenAPHostapdConfiguration(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	deployment := plan.Deployment{
		Name:             "edge",
		WirelessNetworks: []plan.WirelessNetwork{{Name: "home", SSID: "vcpe-lab", Channel: 1, Security: "open"}},
	}
	service := plan.Service{
		Name: "gateway", Type: "gateway",
		Image:   manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Bridges: []manifest.BridgeSpec{{Name: "brlan0"}},
		Instances: []plan.Instance{{Radios: []plan.Radio{{
			Name: "ap", Network: "home", Device: "wlan0", Mode: "ap", Bridge: "brlan0", MAC: "02:00:00:00:00:10",
		}}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{
		Deployment: deployment,
		Service:    service,
		WirelessCredentialFiles: map[string]render.SecretFileHandle{
			"home": {HostPath: "/private/home", ContainerPath: "/run/vcpe/credentials/home"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := make(map[string]string, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	wantConfig := "interface=wlan0\ndriver=nl80211\nssid=vcpe-lab\nhw_mode=g\nchannel=1\nbridge=brlan0\nauth_algs=1\nwpa=0\n"
	if artifacts["hostapd/ap.conf"] != wantConfig {
		t.Fatalf("hostapd config:\n--- got ---\n%s--- want ---\n%s", artifacts["hostapd/ap.conf"], wantConfig)
	}
	if !strings.Contains(artifacts["compose.yaml"], "./hostapd:/etc/vcpe/hostapd:ro") {
		t.Fatalf("compose.yaml does not stage hostapd config:\n%s", artifacts["compose.yaml"])
	}
	if !strings.Contains(artifacts["compose.yaml"], "/private/home:/run/vcpe/credentials/home:ro") {
		t.Fatalf("compose.yaml does not mount the scoped wireless credential:\n%s", artifacts["compose.yaml"])
	}
	for _, want := range []string{"RADIO_AP_DEVICE=wlan0", "RADIO_AP_SSID=vcpe-lab", "RADIO_AP_CHANNEL=1"} {
		if !strings.Contains(artifacts["compose.env"], want) {
			t.Fatalf("compose.env missing %q:\n%s", want, artifacts["compose.env"])
		}
	}
	if !strings.Contains(artifacts["compose.env"], "VCPE_STARTUP_CONTRACT=/etc/vcpe/startup-contract.json") {
		t.Fatalf("compose.env does not configure runtime-init contract:\n%s", artifacts["compose.env"])
	}
	if !strings.Contains(artifacts["compose.yaml"], "../startup-contracts/gateway.json:/etc/vcpe/startup-contract.json:ro") {
		t.Fatalf("compose.yaml does not mount runtime-init contract:\n%s", artifacts["compose.yaml"])
	}
}

func TestGatewayRendersPersonalSecurityPolicyTemplates(t *testing.T) {
	for _, security := range []string{manifest.WirelessWPA2Personal, manifest.WirelessWPA3Personal} {
		t.Run(security, func(t *testing.T) {
			gateway.Register()
			serviceType, _ := typeregistry.Lookup("gateway")
			deployment := plan.Deployment{Name: "edge", WirelessNetworks: []plan.WirelessNetwork{{
				Name: "home", SSID: "vcpe-lab", Channel: 1, Security: security, PassphraseSecretRef: "home-wifi",
			}}}
			service := plan.Service{
				Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
				Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "ap", Network: "home", Device: "wlan0", Mode: "ap", Bridge: "brlan0"}}}},
			}
			result, err := serviceType.Renderer().Render(context.Background(), render.Input{
				Deployment: deployment,
				Service:    service,
				WirelessCredentialFiles: map[string]render.SecretFileHandle{
					"home": {HostPath: "/private/home", ContainerPath: "/run/vcpe/credentials/home"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			artifacts := make(map[string]string, len(result.Artifacts))
			for _, artifact := range result.Artifacts {
				artifacts[artifact.Key] = artifact.Content
			}
			policy := artifacts["hostapd/ap.conf"]
			if !strings.Contains(policy, "# vcpe-security="+security) || !strings.Contains(policy, "# vcpe-passphrase-file=") {
				t.Fatalf("hostapd policy omitted personal-security directives:\n%s", policy)
			}
			if strings.Contains(policy, "home-wifi") || strings.Contains(policy, "wpa_passphrase=") || strings.Contains(policy, "sae_password=") {
				t.Fatalf("hostapd policy exposed credential data or reference:\n%s", policy)
			}
			if !strings.Contains(artifacts["compose.yaml"], "/private/home:/run/vcpe/credentials/home:ro") {
				t.Fatalf("compose.yaml omitted credential mount:\n%s", artifacts["compose.yaml"])
			}
		})
	}
}

func TestGatewayRendersMediaVAPPolicies(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	deployment := plan.Deployment{
		Name: "edge",
		WirelessMedia: []plan.WirelessMedium{
			{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20},
			{Name: "rf6", Band: "6ghz", Channel: 5, WidthMHz: 20},
		},
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "private-home-ref"},
			{Name: "guest", SSID: "Guest", Security: manifest.WirelessOpen},
		},
	}
	service := plan.Service{
		Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
		Bridges: []manifest.BridgeSpec{{Name: "brlan"}, {Name: "brguest"}},
		Instances: []plan.Instance{{Radios: []plan.Radio{
			{Name: "z-ap5", Medium: "rf5", Device: "wlan5", Mode: manifest.RadioModeAP, VAPs: []plan.VAP{
				{Slot: 7, Device: "vap57", MAC: "02:00:00:00:05:07", Network: "guest", Bridge: "brguest"},
				{Slot: 0, Device: "wlan5", MAC: "02:00:00:00:05:00", Network: "home", Bridge: "brlan"},
			}},
			{Name: "a-ap6", Medium: "rf6", Device: "wlan6", Mode: manifest.RadioModeAP, VAPs: []plan.VAP{
				{Slot: 0, Device: "wlan6", MAC: "02:00:00:00:06:00", Network: "home", Bridge: "brlan"},
			}},
		}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: deployment, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		if strings.HasPrefix(artifact.Key, "hostapd/") {
			keys = append(keys, artifact.Key)
			artifacts[artifact.Key] = artifact.Content
		}
	}
	if got := strings.Join(keys, ","); got != "hostapd/a-ap6.conf,hostapd/z-ap5.conf" {
		t.Fatalf("AP policies must be sorted by radio name: %v", keys)
	}
	want5 := "interface=wlan5\ndriver=nl80211\nhw_mode=a\nchannel=36\n" +
		"bssid=02:00:00:00:05:00\nssid=Home\nbridge=brlan\nauth_algs=1\n" +
		"# vcpe-security=wpa3-personal\n# vcpe-passphrase-file=" + secrets.WirelessCredentialContainerPath("home") + "\n" +
		"bss=vap57\nbssid=02:00:00:00:05:07\nssid=Guest\nbridge=brguest\nauth_algs=1\nwpa=0\n"
	if got := artifacts["hostapd/z-ap5.conf"]; got != want5 {
		t.Fatalf("5 GHz multi-BSS policy:\n--- got ---\n%s--- want ---\n%s", got, want5)
	}
	policy6 := artifacts["hostapd/a-ap6.conf"]
	for _, want := range []string{"hw_mode=a\nchannel=5\n", "country_code=US\ncountry3=0x49\nieee80211d=1\nop_class=131\nieee80211ax=1\nhe_oper_chwidth=0\nhe_6ghz_reg_pwr_type=0\n", "bssid=02:00:00:00:06:00", "sae_pwe=1\n"} {
		if !strings.Contains(policy6, want) {
			t.Fatalf("6 GHz policy missing %q:\n%s", want, policy6)
		}
	}
	for _, policy := range artifacts {
		if strings.Contains(policy, "private-home-ref") || strings.Contains(policy, "sae_password=") || strings.Contains(policy, "wpa_passphrase=") {
			t.Fatalf("hostapd policy exposed secret material: %s", policy)
		}
	}
}

func TestGatewayRendersEightMixedPolicyVAPs(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	deployment := plan.Deployment{
		Name:          "edge",
		WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"},
			{Name: "guest", SSID: "Guest", Security: manifest.WirelessOpen},
			{Name: "iot", SSID: "IoT", Security: manifest.WirelessWPA2Personal, PassphraseSecretRef: "iot-ref"},
		},
	}
	service := plan.Service{
		Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
		Instances: []plan.Instance{{Radios: []plan.Radio{{
			Name: "ap5", Medium: "rf5", Device: "wlan0", Mode: manifest.RadioModeAP,
			VAPs: []plan.VAP{
				{Slot: 7, Device: "vap7", MAC: "02:00:00:00:05:07", Network: "guest", Bridge: "brguest"},
				{Slot: 6, Device: "vap6", MAC: "02:00:00:00:05:06", Network: "home", Bridge: "brlan"},
				{Slot: 5, Device: "vap5", MAC: "02:00:00:00:05:05", Network: "iot", Bridge: "briot"},
				{Slot: 4, Device: "vap4", MAC: "02:00:00:00:05:04", Network: "guest", Bridge: "brguest"},
				{Slot: 3, Device: "vap3", MAC: "02:00:00:00:05:03", Network: "home", Bridge: "brlan"},
				{Slot: 2, Device: "vap2", MAC: "02:00:00:00:05:02", Network: "iot", Bridge: "briot"},
				{Slot: 1, Device: "vap1", MAC: "02:00:00:00:05:01", Network: "guest", Bridge: "brguest"},
				{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:05:00", Network: "home", Bridge: "brlan"},
			},
		}}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: deployment, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	var policy string
	for _, artifact := range result.Artifacts {
		if artifact.Key == "hostapd/ap5.conf" {
			policy = artifact.Content
		}
	}
	wpa3 := "# vcpe-security=wpa3-personal\n# vcpe-passphrase-file=" + secrets.WirelessCredentialContainerPath("home") + "\n"
	wpa2 := "# vcpe-security=wpa2-personal\n# vcpe-passphrase-file=" + secrets.WirelessCredentialContainerPath("iot") + "\n"
	want := "interface=wlan0\ndriver=nl80211\nhw_mode=a\nchannel=36\n" +
		"bssid=02:00:00:00:05:00\nssid=Home\nbridge=brlan\nauth_algs=1\n" + wpa3 +
		"bss=vap1\nbssid=02:00:00:00:05:01\nssid=Guest\nbridge=brguest\nauth_algs=1\nwpa=0\n" +
		"bss=vap2\nbssid=02:00:00:00:05:02\nssid=IoT\nbridge=briot\nauth_algs=1\n" + wpa2 +
		"bss=vap3\nbssid=02:00:00:00:05:03\nssid=Home\nbridge=brlan\nauth_algs=1\n" + wpa3 +
		"bss=vap4\nbssid=02:00:00:00:05:04\nssid=Guest\nbridge=brguest\nauth_algs=1\nwpa=0\n" +
		"bss=vap5\nbssid=02:00:00:00:05:05\nssid=IoT\nbridge=briot\nauth_algs=1\n" + wpa2 +
		"bss=vap6\nbssid=02:00:00:00:05:06\nssid=Home\nbridge=brlan\nauth_algs=1\n" + wpa3 +
		"bss=vap7\nbssid=02:00:00:00:05:07\nssid=Guest\nbridge=brguest\nauth_algs=1\nwpa=0\n"
	if policy != want {
		t.Fatalf("eight-slot policy:\n--- got ---\n%s--- want ---\n%s", policy, want)
	}
	if strings.Contains(policy, "home-ref") || strings.Contains(policy, "iot-ref") {
		t.Fatalf("policy disclosed credential reference: %s", policy)
	}
}

func TestGeneratedMultiBSSArtifactsAreAccepted(t *testing.T) {
	podmanCompose, err := exec.LookPath("podman-compose")
	if err != nil {
		t.Skip("podman-compose is required for generated Compose validation")
	}
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	deployment := plan.Deployment{
		Name: "edge", WirelessMedia: []plan.WirelessMedium{{Name: "rf24", Band: "2.4ghz", Channel: 1, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{{Name: "guest", SSID: "vcpe-guest", Security: manifest.WirelessOpen}},
	}
	vaps := make([]plan.VAP, 8)
	for slot := range vaps {
		device := "wlan0"
		if slot > 0 {
			device = fmt.Sprintf("wlan0v%d", slot)
		}
		vaps[slot] = plan.VAP{Slot: slot, Device: device, MAC: fmt.Sprintf("02:76:63:70:24:%02x", slot+16), Network: "guest", Bridge: "brguest"}
	}
	service := plan.Service{
		Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
		Bridges:   []manifest.BridgeSpec{{Name: "brguest"}},
		Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "ap24", Medium: "rf24", Device: "wlan0", Mode: manifest.RadioModeAP, VAPs: vaps}}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: deployment, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	artifactDir := t.TempDir()
	for _, artifact := range result.Artifacts {
		path := filepath.Join(artifactDir, artifact.Key)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(artifact.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	compose := exec.Command(podmanCompose, "-f", filepath.Join(artifactDir, "compose.yaml"), "config")
	compose.Dir = artifactDir
	if output, err := compose.CombinedOutput(); err != nil {
		t.Fatalf("generated Gateway Compose: %v\n%s", err, output)
	}
	image := os.Getenv("VCPE_GATEWAY_IMAGE")
	if image == "" {
		t.Skip("set VCPE_GATEWAY_IMAGE for pinned hostapd policy acceptance")
	}
	if _, err := exec.LookPath("podman"); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(artifactDir, "hostapd", "ap24.conf")
	cmd := exec.Command("podman", "run", "--rm", "--pull=never", "--entrypoint", "/usr/sbin/hostapd",
		"-v", policy+":/tmp/ap.conf:ro", image, "/tmp/ap.conf")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Could not read interface wlan0 flags") {
		t.Fatalf("generated multi-BSS policy did not reach nl80211 device lookup: %v\n%s", err, output)
	}
}

func TestGatewayRenders24GHzWPA2VAPPolicy(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	deployment := plan.Deployment{
		Name:             "edge",
		WirelessMedia:    []plan.WirelessMedium{{Name: "rf24", Band: "2.4ghz", Channel: 6, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{{Name: "iot", SSID: "IoT", Security: manifest.WirelessWPA2Personal, PassphraseSecretRef: "iot-ref"}},
	}
	service := plan.Service{
		Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"},
		Instances: []plan.Instance{{Radios: []plan.Radio{{
			Name: "ap24", Medium: "rf24", Device: "wlan0", Mode: manifest.RadioModeAP,
			VAPs: []plan.VAP{{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:24:00", Network: "iot", Bridge: "briot"}},
		}}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: deployment, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Key != "hostapd/ap24.conf" {
			continue
		}
		policy := artifact.Content
		for _, want := range []string{"interface=wlan0\n", "hw_mode=g\nchannel=6\n", "bssid=02:00:00:00:24:00\n", "ssid=IoT\nbridge=briot\n", "# vcpe-security=wpa2-personal\n", "# vcpe-passphrase-file=" + secrets.WirelessCredentialContainerPath("iot") + "\n"} {
			if !strings.Contains(policy, want) {
				t.Fatalf("2.4 GHz policy missing %q:\n%s", want, policy)
			}
		}
		return
	}
	t.Fatal("2.4 GHz AP policy was not rendered")
}

func TestGatewayWithoutRadiosDoesNotStageHostapd(t *testing.T) {
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	service := plan.Service{
		Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range result.Artifacts {
		if strings.HasPrefix(artifact.Key, "hostapd/") || strings.Contains(artifact.Content, "/etc/vcpe/hostapd") || strings.Contains(artifact.Content, "/run/vcpe/credentials") {
			t.Fatalf("unexpected hostapd artifact without radios: %#v", artifact)
		}
	}
}

func TestGatewayPublishesHealthDirectlyWithoutManagedTopology(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Addressing: "static"},
		}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	compose := composeArtifact(t, result)
	for _, want := range []string{"127.0.0.1:47000:9878", "aa-health:"} {
		if !strings.Contains(compose, want) {
			t.Fatalf("expected direct health publication to contain %q:\n%s", want, compose)
		}
	}
	if strings.Contains(compose, "vcpe-healthd") || strings.Contains(compose, "gateway-1-health") {
		t.Fatalf("expected no per-instance health proxy service:\n%s", compose)
	}
}

func TestGatewayPublishesHealthDirectlyWithManagedTopology(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0"},
		}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	compose := composeArtifact(t, result)
	if !strings.Contains(compose, "127.0.0.1:47000:9878") {
		t.Fatalf("expected the workload's own health mapping:\n%s", compose)
	}
	if strings.Contains(compose, "aa-health:") || strings.Contains(compose, "vcpe-healthd") {
		t.Fatalf("expected no private health network or proxy when the topology attachment is already Podman-managed:\n%s", compose)
	}
}

func composeArtifact(t *testing.T, result render.Result) string {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			return artifact.Content
		}
	}
	t.Fatal("no compose.yaml artifact produced")
	return ""
}

func TestGatewayAddressingReflectedInEnv(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan"}, {Role: "cm", Bridge: "edge-cm"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Gateway4: "10.7.200.1", Addressing: "static"},
			{Role: "cm", Network: "edge-cm", Device: "wan0"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	got := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			got = a.Content
		}
	}
	if !strings.Contains(got, "IFACE_WAN_ADDRESSING=static") {
		t.Fatalf("compose.env missing IFACE_WAN_ADDRESSING=static:\n%s", got)
	}
	if !strings.Contains(got, "IFACE_CM_ADDRESSING=dhcp") {
		t.Fatalf("compose.env missing IFACE_CM_ADDRESSING=dhcp (default):\n%s", got)
	}
}

// TestGatewayDHCPAddressingCoexistsWithHealthPublication verifies a dhcp-
// addressed interface and direct health publication coexist without
// conflict, since publication forwards through the managed aa-health
// attachment by loopback port rather than by this interface's resolved
// address.
func TestGatewayDHCPAddressingCoexistsWithHealthPublication(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway with dhcp addressing: %v", err)
	}
	compose := composeArtifact(t, result)
	if !strings.Contains(compose, "aa-health:") {
		t.Fatalf("expected direct health publication even with addressing: dhcp on the self-addressed interface:\n%s", compose)
	}
}

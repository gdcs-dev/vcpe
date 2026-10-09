package render_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
)

func TestSortedEnv(t *testing.T) {
	values := map[string]string{"ZED": "last", "ALPHA": "first", "EMPTY": ""}
	before := map[string]string{"ZED": "last", "ALPHA": "first", "EMPTY": ""}
	if got, want := render.SortedEnv(values), []string{"ALPHA=first", "EMPTY=", "ZED=last"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SortedEnv() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(values, before) {
		t.Errorf("SortedEnv mutated input: got %#v, want %#v", values, before)
	}
	if got := render.SortedEnv(nil); len(got) != 0 {
		t.Errorf("SortedEnv(nil) = %#v, want empty", got)
	}
}

func TestInstanceEnvArtifacts(t *testing.T) {
	input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 2}, {Index: 0}}}}
	var called []int
	artifacts := render.InstanceEnvArtifacts(input, func(instance plan.Instance) []string {
		called = append(called, instance.Index)
		return []string{"INSTANCE=" + string(rune('0'+instance.Index))}
	})
	want := []render.Artifact{
		{Key: "compose.env", Content: "INSTANCE=2\n"},
		{Key: "instances/3/compose.env", Content: "INSTANCE=2\n"},
		{Key: "instances/1/compose.env", Content: "INSTANCE=0\n"},
	}
	if !reflect.DeepEqual(artifacts, want) {
		t.Errorf("InstanceEnvArtifacts() = %#v, want %#v", artifacts, want)
	}
	if !reflect.DeepEqual(called, []int{2, 0}) {
		t.Errorf("callback order = %#v, want plan order", called)
	}
	if got := render.InstanceEnvArtifacts(render.Input{}, func(plan.Instance) []string { return nil }); len(got) != 0 {
		t.Errorf("zero-instance artifacts = %#v, want empty", got)
	}
}

func TestImageRefCurrentBehavior(t *testing.T) {
	tests := []struct {
		name  string
		image manifest.Image
		want  string
	}{
		{name: "empty repository", want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example/workload"}, want: "example/workload:latest"},
		{name: "whitespace-only tag", image: manifest.Image{Repository: "example/workload", Tag: "  "}, want: "example/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example/workload", Tag: "v2"}, want: "example/workload:v2"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := render.ImageRef(testCase.image); got != testCase.want {
				t.Errorf("ImageRef(%+v) = %q, want %q", testCase.image, got, testCase.want)
			}
		})
	}
}

func TestIPWithPrefix(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		cidr string
		want string
	}{
		{name: "empty IP", cidr: "10.1.2.0/24", want: ""},
		{name: "empty CIDR", ip: "10.1.2.3", want: "10.1.2.3"},
		{name: "invalid CIDR", ip: "10.1.2.3", cidr: "not-a-cidr", want: "10.1.2.3"},
		{name: "IPv4", ip: "10.1.2.3", cidr: "10.1.2.0/24", want: "10.1.2.3/24"},
		{name: "IPv6", ip: "2001:db8::3", cidr: "2001:db8::/64", want: "2001:db8::3/64"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := render.IPWithPrefix(testCase.ip, testCase.cidr); got != testCase.want {
				t.Errorf("IPWithPrefix(%q, %q) = %q, want %q", testCase.ip, testCase.cidr, got, testCase.want)
			}
		})
	}
}

func TestIfaceEnvDefaultRoute(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "lan-p1", Device: "eth0", DefaultRoute: true},
			{Role: "mgmt", Device: "mgmt0"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_LAN_P1_DEFAULT_ROUTE=1") {
		t.Errorf("expected IFACE_LAN_P1_DEFAULT_ROUTE=1 in env:\n%s", env)
	}
	if strings.Contains(env, "IFACE_MGMT_DEFAULT_ROUTE") {
		t.Errorf("expected no IFACE_MGMT_DEFAULT_ROUTE in env:\n%s", env)
	}
}

func TestIfaceEnvAddressing(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "wan", Device: "eth0", Addressing: "static"},
			{Role: "mgmt", Device: "eth1"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_WAN_ADDRESSING=static") {
		t.Errorf("expected IFACE_WAN_ADDRESSING=static in env:\n%s", env)
	}
	if !strings.Contains(env, "IFACE_MGMT_ADDRESSING=dhcp") {
		t.Errorf("expected IFACE_MGMT_ADDRESSING=dhcp (default) in env:\n%s", env)
	}
}

func TestIfaceEnvNetworkManaged(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "mgmt", Device: "eth0", ManagedNetwork: true},
			{Role: "wan", Device: "eth1"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_MGMT_NETWORK_MANAGED=1") {
		t.Errorf("expected IFACE_MGMT_NETWORK_MANAGED=1 in env:\n%s", env)
	}
	if strings.Contains(env, "IFACE_WAN_NETWORK_MANAGED") {
		t.Errorf("expected no IFACE_WAN_NETWORK_MANAGED for an unmanaged network in env:\n%s", env)
	}
}

func TestRadioEnvIsCompleteAndOrdered(t *testing.T) {
	dep := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "guest", SSID: "vcpe guest", Channel: 11, Security: "open"},
		},
	}
	inst := plan.Instance{Radios: []plan.Radio{
		{Name: "guest-station", Network: "guest", Device: "wlan1", Mode: "station", MAC: "02:00:00:00:00:02", Addressing: "dhcp", DefaultRoute: true},
		{Name: "guest-ap", Network: "guest", Device: "wlan0", Mode: "ap", MAC: "02:00:00:00:00:01", Bridge: "brlan0"},
	}}
	want := []string{
		"RADIO_GUEST_AP_ADDRESSING=",
		"RADIO_GUEST_AP_BRIDGE=brlan0",
		"RADIO_GUEST_AP_CHANNEL=11",
		"RADIO_GUEST_AP_DEFAULT_ROUTE=0",
		"RADIO_GUEST_AP_DEVICE=wlan0",
		"RADIO_GUEST_AP_MAC=02:00:00:00:00:01",
		"RADIO_GUEST_AP_MODE=ap",
		"RADIO_GUEST_AP_NETWORK=guest",
		"RADIO_GUEST_AP_SSID=vcpe guest",
		"RADIO_GUEST_STATION_ADDRESSING=dhcp",
		"RADIO_GUEST_STATION_BRIDGE=",
		"RADIO_GUEST_STATION_CHANNEL=11",
		"RADIO_GUEST_STATION_DEFAULT_ROUTE=1",
		"RADIO_GUEST_STATION_DEVICE=wlan1",
		"RADIO_GUEST_STATION_MAC=02:00:00:00:00:02",
		"RADIO_GUEST_STATION_MODE=station",
		"RADIO_GUEST_STATION_NETWORK=guest",
		"RADIO_GUEST_STATION_SSID=vcpe guest",
	}
	if got := render.RadioEnv(dep, inst); !reflect.DeepEqual(got, want) {
		t.Fatalf("RadioEnv() = %#v, want %#v", got, want)
	}

	withBase := render.IfaceEnv(dep, plan.Service{Name: "client"}, inst)
	if !reflect.DeepEqual(withBase[3:], want) {
		t.Fatalf("IfaceEnv radio tail = %#v, want %#v", withBase[3:], want)
	}
}

func TestRadioEnvIncludesPersonalSecurityFilePolicy(t *testing.T) {
	dep := plan.Deployment{WirelessNetworks: []plan.WirelessNetwork{{
		Name: "home", SSID: "vcpe-lab", Channel: 1, Security: manifest.WirelessWPA3Personal,
	}}}
	inst := plan.Instance{Radios: []plan.Radio{{Name: "wifi", Network: "home"}}}
	env := strings.Join(render.RadioEnv(dep, inst), "\n")
	if !strings.Contains(env, "RADIO_WIFI_SECURITY=wpa3-personal") {
		t.Fatalf("radio env omitted security policy:\n%s", env)
	}
	if !strings.Contains(env, "RADIO_WIFI_PASSPHRASE_FILE="+secrets.WirelessCredentialContainerPath("home")) {
		t.Fatalf("radio env omitted credential file path:\n%s", env)
	}
	if strings.Contains(env, "passphraseSecretRef") {
		t.Fatalf("radio env exposed manifest secret reference:\n%s", env)
	}
}

func TestRadioEnvRendersMediaAndOrderedVAPPolicy(t *testing.T) {
	dep := plan.Deployment{
		WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "private-secret"},
			{Name: "guest", SSID: "Guest", Security: manifest.WirelessOpen},
		},
	}
	inst := plan.Instance{Radios: []plan.Radio{
		{Name: "ap5", Medium: "rf5", Mode: manifest.RadioModeAP, Device: "wlan0", MAC: "02:00:00:00:00:01", VAPs: []plan.VAP{
			{Slot: 7, Device: "vap7", MAC: "02:00:00:00:00:07", Network: "guest", Bridge: "brguest"},
			{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:00:01", Network: "home", Bridge: "brlan"},
		}},
		{Name: "sta5", Medium: "rf5", Mode: manifest.RadioModeStation, Device: "wlan1", Network: "home", APBSSID: "02:00:00:00:00:01", Addressing: manifest.AddressingDHCP},
	}}
	first := render.RadioEnv(dep, inst)
	if !reflect.DeepEqual(first, render.RadioEnv(dep, inst)) {
		t.Fatal("media radio env changed between calls")
	}
	env := strings.Join(first, "\n")
	for _, field := range []string{
		"RADIO_AP5_MEDIUM=rf5", "RADIO_AP5_BAND=5ghz", "RADIO_AP5_CHANNEL=36", "RADIO_AP5_WIDTH_MHZ=20",
		"RADIO_AP5_VAP_0_DEVICE=wlan0", "RADIO_AP5_VAP_0_BSSID=02:00:00:00:00:01",
		"RADIO_AP5_VAP_0_BRIDGE=brlan", "RADIO_AP5_VAP_0_SSID=Home", "RADIO_AP5_VAP_0_SECURITY=wpa3-personal",
		"RADIO_AP5_VAP_0_PASSPHRASE_FILE=" + secrets.WirelessCredentialContainerPath("home"),
		"RADIO_AP5_VAP_7_DEVICE=vap7", "RADIO_AP5_VAP_7_BRIDGE=brguest", "RADIO_AP5_VAP_7_SECURITY=open",
		"RADIO_STA5_MEDIUM=rf5", "RADIO_STA5_NETWORK=home", "RADIO_STA5_SSID=Home", "RADIO_STA5_AP_BSSID=02:00:00:00:00:01",
		"RADIO_STA5_PASSPHRASE_FILE=" + secrets.WirelessCredentialContainerPath("home"),
	} {
		if !strings.Contains(env, field) {
			t.Errorf("radio env missing %q:\n%s", field, env)
		}
	}
	if strings.Contains(env, "private-secret") || strings.Contains(env, "RADIO_AP5_VAP_7_PASSPHRASE_FILE") ||
		strings.Index(env, "RADIO_AP5_VAP_0_") > strings.Index(env, "RADIO_AP5_VAP_7_") {
		t.Fatalf("radio env leaked a reference, gave open VAP a credential, or misordered slots:\n%s", env)
	}
}

func TestRadioEnvWithoutRadiosIsEmpty(t *testing.T) {
	if got := render.RadioEnv(plan.Deployment{}, plan.Instance{}); len(got) != 0 {
		t.Fatalf("RadioEnv() = %#v, want empty", got)
	}
}

func TestRadioEnvRendersMeshAndRoamingContracts(t *testing.T) {
	dep := plan.Deployment{WirelessMedia: []plan.WirelessMedium{{Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}}}
	inst := plan.Instance{Radios: []plan.Radio{
		{Name: "backhaul", Medium: "rf5", Mode: manifest.RadioModeMesh, Mesh: &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-secret"}},
		{Name: "station", Medium: "rf5", Mode: manifest.RadioModeStation, RoamingMedia: []string{"rf5"}, Candidates: []plan.RoamingCandidate{
			{Service: "gateway", Replica: 0, Radio: "ap", Slot: 0, Medium: "rf5", Band: "5ghz", Channel: 36, BSSID: "02:00:00:00:00:01"},
			{Service: "remote", Replica: 0, Radio: "ap", Slot: 0, Medium: "rf5", Band: "5ghz", Channel: 36, BSSID: "02:00:00:00:00:02"},
		}},
	}}
	env := strings.Join(render.RadioEnv(dep, inst), "\n")
	for _, value := range []string{
		"RADIO_BACKHAUL_MESH_ID=mesh-home", "RADIO_BACKHAUL_BRIDGE=brlan",
		"RADIO_BACKHAUL_SAE_FILE=" + secrets.MeshCredentialContainerPath("mesh-home"),
		"RADIO_STATION_CANDIDATE_COUNT=2", "RADIO_STATION_CANDIDATE_0_BSSID=02:00:00:00:00:01",
		"RADIO_STATION_CANDIDATE_0_CHANNEL=36", "RADIO_STATION_CANDIDATE_1_BSSID=02:00:00:00:00:02",
	} {
		if !strings.Contains(env, value) {
			t.Errorf("radio env missing %q:\n%s", value, env)
		}
	}
	if strings.Contains(env, "mesh-secret") || strings.Contains(env, "RADIO_STATION_AP_BSSID=") {
		t.Fatalf("env exposed mesh secret or pinned roaming station:\n%s", env)
	}
}

package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
)

func TestWirelessCredentialFilesAreScopedToServiceConsumers(t *testing.T) {
	stateRoot := t.TempDir()
	deployment := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-wifi"},
			{Name: "guest", Security: manifest.WirelessOpen},
		},
	}
	consumer := plan.Service{Instances: []plan.Instance{{Radios: []plan.Radio{
		{Network: "home"},
		{Network: "guest"},
	}}}}

	handles := wirelessCredentialFiles(stateRoot, deployment, consumer)
	if len(handles) != 1 {
		t.Fatalf("consumer handles = %+v", handles)
	}
	handle := handles["home"]
	if handle.HostPath != secrets.WirelessCredentialPath(stateRoot, "edge", "home") {
		t.Fatalf("host path = %q", handle.HostPath)
	}
	if handle.ContainerPath != secrets.WirelessCredentialContainerPath("home") {
		t.Fatalf("container path = %q", handle.ContainerPath)
	}

	if unrelated := wirelessCredentialFiles(stateRoot, deployment, plan.Service{}); len(unrelated) != 0 {
		t.Fatalf("unrelated service handles = %+v", unrelated)
	}
}

func TestWirelessCredentialFilesIncludeMirroredVAPProfilesOnce(t *testing.T) {
	stateRoot := t.TempDir()
	deployment := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{
			{Name: "home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"},
			{Name: "iot", Security: manifest.WirelessWPA2Personal, PassphraseSecretRef: "iot-ref"},
			{Name: "guest", Security: manifest.WirelessOpen},
		},
	}
	radios := []plan.Radio{
		{Medium: "rf24", VAPs: []plan.VAP{{Slot: 0, Network: "home"}, {Slot: 1, Network: "iot"}, {Slot: 2, Network: "guest"}}},
		{Medium: "rf5", VAPs: []plan.VAP{{Slot: 0, Network: "home"}}},
	}
	service := plan.Service{Instances: []plan.Instance{{Radios: radios}, {Radios: radios}}}
	handles := wirelessCredentialFiles(stateRoot, deployment, service)
	if len(handles) != 2 {
		t.Fatalf("mirrored VAP profiles should mount once each, excluding open policy: %+v", handles)
	}
	for _, profile := range []string{"home", "iot"} {
		handle := handles[profile]
		if handle.HostPath != secrets.WirelessCredentialPath(stateRoot, "edge", profile) ||
			handle.ContainerPath != secrets.WirelessCredentialContainerPath(profile) {
			t.Fatalf("profile %q mount = %+v", profile, handle)
		}
	}
	station := plan.Service{Instances: []plan.Instance{{Radios: []plan.Radio{{Medium: "rf5", Network: "home"}}}}}
	if stationHandles := wirelessCredentialFiles(stateRoot, deployment, station); len(stationHandles) != 1 || stationHandles["home"].ContainerPath != secrets.WirelessCredentialContainerPath("home") {
		t.Fatalf("station mounts = %+v", stationHandles)
	}
	if guest := wirelessCredentialFiles(stateRoot, deployment, plan.Service{Instances: []plan.Instance{{Radios: []plan.Radio{{Medium: "rf24", Network: "guest"}}}}}); len(guest) != 0 {
		t.Fatalf("open profile mounted a credential: %+v", guest)
	}
}

func TestMirroredProfileSentinelStaysInProtectedCredential(t *testing.T) {
	const sentinel = "SENTINEL-mirrored-profile-passphrase"
	stateRoot := t.TempDir()
	document := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec:     manifest.Spec{WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"}}},
	}
	deployment := plan.Deployment{
		Name:             "edge",
		WirelessMedia:    []plan.WirelessMedium{{Name: "rf24", Band: "2.4ghz", Channel: 1, WidthMHz: 20}, {Name: "rf5", Band: "5ghz", Channel: 36, WidthMHz: 20}},
		WirelessNetworks: []plan.WirelessNetwork{{Name: "home", SSID: "Home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"}},
		Services: []plan.Service{
			{Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: "gateway"}, Instances: []plan.Instance{{Radios: []plan.Radio{
				{Name: "ap24", Medium: "rf24", Mode: manifest.RadioModeAP, Device: "wlan0", VAPs: []plan.VAP{{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:24:00", Network: "home", Bridge: "brlan"}}},
				{Name: "ap5", Medium: "rf5", Mode: manifest.RadioModeAP, Device: "wlan1", VAPs: []plan.VAP{{Slot: 0, Device: "wlan1", MAC: "02:00:00:00:05:00", Network: "home", Bridge: "brlan"}}},
			}}}},
			{Name: "unrelated", Instances: []plan.Instance{{}}},
		},
	}
	if _, err := prepareWirelessCredentials(stateRoot, manifest.Document{}, false, document, deployment, map[string]string{"home-ref": sentinel}); err != nil {
		t.Fatal(err)
	}
	protected, err := os.ReadFile(secrets.WirelessCredentialPath(stateRoot, "edge", "home"))
	if err != nil || string(protected) != sentinel {
		t.Fatalf("protected credential absent: %v", err)
	}
	if handles := wirelessCredentialFiles(stateRoot, deployment, deployment.Services[1]); len(handles) != 0 {
		t.Fatalf("unrelated service received protected files: %v", handles)
	}
	gateway.Register()
	serviceType, _ := typeregistry.Lookup("gateway")
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{
		Deployment: deployment, Service: deployment.Services[0],
		WirelessCredentialFiles: wirelessCredentialFiles(stateRoot, deployment, deployment.Services[0]),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range result.Artifacts {
		if strings.Contains(artifact.Content, sentinel) {
			t.Fatalf("credential leaked in %s", artifact.Key)
		}
	}
	for _, built := range contract.BuildForDeployment("operation", deployment) {
		encoded, err := json.Marshal(built)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), sentinel) {
			t.Fatal("credential leaked in startup contract")
		}
	}
}

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

type readinessInspectorStub struct {
	ready        bool
	vapStatus    []byte
	vapError     error
	meshStatus   []byte
	clientStatus []byte
	reachable    bool
}

func (stub readinessInspectorStub) ContainerMeshStatus(context.Context, string) ([]byte, error) {
	return stub.meshStatus, nil
}

func TestWaitForGatewayMeshesRequiresPeerAndBridge(t *testing.T) {
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{{Name: "remote", Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "backhaul", Device: "mesh0", Mode: manifest.RadioModeMesh, Bridge: "brlan", Mesh: &manifest.Mesh{ID: "mesh-home", Bridge: "brlan"}}}}}}}}
	for _, test := range []struct {
		name   string
		status string
		ready  bool
	}{
		{"established", `{"radio":"backhaul","interface":"mesh0","bridge":"brlan","joined":true,"peers":1}` + "\n", true},
		{"no peer", `{"radio":"backhaul","interface":"mesh0","bridge":"brlan","joined":true,"peers":0}` + "\n", false},
		{"wrong bridge", `{"radio":"backhaul","interface":"mesh0","bridge":"bypass","joined":true,"peers":1}` + "\n", false},
		{"not joined", `{"radio":"backhaul","interface":"mesh0","bridge":"brlan","joined":false,"peers":1}` + "\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalTimeout, originalPoll := wirelessMeshTimeout, wirelessMeshPoll
			wirelessMeshTimeout, wirelessMeshPoll = 5*time.Millisecond, time.Millisecond
			t.Cleanup(func() { wirelessMeshTimeout, wirelessMeshPoll = originalTimeout, originalPoll })
			err := waitForGatewayMeshes(context.Background(), deployment, readinessInspectorStub{meshStatus: []byte(test.status)})
			if test.ready && err != nil || !test.ready && (err == nil || !strings.Contains(err.Error(), "remote-1 backhaul")) {
				t.Fatalf("mesh readiness = %v, ready=%t", err, test.ready)
			}
		})
	}
}

func (stub readinessInspectorStub) ContainerFile(context.Context, string, string) ([]byte, error) {
	if stub.clientStatus != nil {
		return stub.clientStatus, nil
	}
	if stub.ready {
		return []byte("state=ready\n"), nil
	}
	return nil, errors.New("not ready")
}

func (stub readinessInspectorStub) ProbeLAN(context.Context, string, string, string) error {
	if !stub.reachable {
		return errors.New("no path to root")
	}
	return nil
}

func TestWaitForRemoteMeshDHCPRequiresRootLeaseAndPath(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	bssid := deployment.WirelessScenarios[0].Assertions.FinalAP.BSSID
	status := "radio=WIFI\ndevice=wlan0\nbssid=" + bssid + "\nipv4=10.0.0.100/24\nstate=ready\n"
	originalTimeout, originalPoll := wirelessMeshTimeout, wirelessMeshPoll
	wirelessMeshTimeout, wirelessMeshPoll = 5*time.Millisecond, time.Millisecond
	t.Cleanup(func() { wirelessMeshTimeout, wirelessMeshPoll = originalTimeout, originalPoll })
	for _, test := range []struct {
		name       string
		stub       readinessInspectorStub
		wantReady  bool
		wantDetail string
	}{
		{"remote lease and path", readinessInspectorStub{clientStatus: []byte(status), reachable: true}, true, ""},
		{"root unreachable", readinessInspectorStub{clientStatus: []byte(status)}, false, "LAN probe failed"},
		{"wrong AP", readinessInspectorStub{clientStatus: []byte(strings.Replace(status, bssid, "02:00:00:00:00:01", 1)), reachable: true}, false, "expected AP"},
		{"no root lease", readinessInspectorStub{clientStatus: []byte(strings.Replace(status, "10.0.0.100/24", "10.0.1.100/24", 1)), reachable: true}, false, "lease outside root DHCP range"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := waitForRemoteMeshDHCP(context.Background(), deployment, test.stub)
			if test.wantReady && err != nil || !test.wantReady && (err == nil || !strings.Contains(err.Error(), "remote-probe") || !strings.Contains(err.Error(), test.wantDetail)) {
				t.Fatalf("remote DHCP readiness = %v, wantReady = %t", err, test.wantReady)
			}
		})
	}
}

func (stub readinessInspectorStub) ContainerVAPStatus(_ context.Context, container string) ([]byte, error) {
	if stub.vapStatus == nil && stub.vapError == nil {
		deployment := strings.TrimSuffix(container, "-gateway-1")
		return []byte(fmt.Sprintf("{\"radio\":\"ap\",\"slot\":0,\"interface\":\"wlan0\",\"bssid\":%q,\"enabled\":true,\"bridge\":\"brlan0\"}\n",
			plan.CanonicalVAPBSSID(deployment, "gateway", 0, "ap", 0))), nil
	}
	return stub.vapStatus, stub.vapError
}

func TestWaitForGatewayVAPsRequiresEveryPlannedSlot(t *testing.T) {
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{{
		Name: "gateway", Instances: []plan.Instance{{Radios: []plan.Radio{{
			Name: "ap5", Mode: manifest.RadioModeAP, Medium: "rf5", VAPs: []plan.VAP{
				{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:00:10", Bridge: "brlan"},
				{Slot: 7, Device: "vap7", MAC: "02:00:00:00:00:17", Bridge: "brguest"},
			},
		}}}},
	}}}
	const first = `{"radio":"ap5","slot":0,"interface":"wlan0","bssid":"02:00:00:00:00:10","enabled":true,"bridge":"brlan"}` + "\n"
	const second = `{"radio":"ap5","slot":7,"interface":"vap7","bssid":"02:00:00:00:00:17","enabled":true,"bridge":"brguest"}` + "\n"
	for _, test := range []struct {
		name       string
		status     string
		ready      bool
		probeError error
	}{
		{"all ready", first + second, true, nil},
		{"missing secondary", first, false, nil},
		{"failed secondary probe", first, false, errors.New("exit status 1")},
		{"wrong BSSID", first + strings.Replace(second, "00:17", "00:18", 1), false, nil},
		{"wrong bridge", first + strings.Replace(second, "brguest", "brwrong", 1), false, nil},
		{"disabled", first + strings.Replace(second, `"enabled":true`, `"enabled":false`, 1), false, nil},
		{"nonzero complete probe", first + second, false, errors.New("exit status 1")},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalTimeout, originalPoll := wirelessVAPTimeout, wirelessVAPPoll
			wirelessVAPTimeout, wirelessVAPPoll = 5*time.Millisecond, time.Millisecond
			t.Cleanup(func() { wirelessVAPTimeout, wirelessVAPPoll = originalTimeout, originalPoll })
			err := waitForGatewayVAPs(context.Background(), deployment, readinessInspectorStub{vapStatus: []byte(test.status), vapError: test.probeError})
			if test.ready && err != nil || !test.ready && (err == nil || !strings.Contains(err.Error(), "ap5 slot 7")) {
				t.Fatalf("readiness error = %v, ready = %t", err, test.ready)
			}
		})
	}
}

func TestWaitForWirelessAuthenticationSelectsPersonalWirelessClient(t *testing.T) {
	deployment := credentialPlan()
	deployment.Services[1].Image.Repository = "ghcr.io/gdcs-dev/wireless-client"
	deployment.WirelessNetworks = []plan.WirelessNetwork{{Name: "home", Security: manifest.WirelessWPA3Personal}}
	for index := range deployment.Services[1].Instances {
		deployment.Services[1].Instances[index].Radios[0].Mode = "station"
	}
	containers := personalWirelessClientContainers(deployment)
	if strings.Join(containers, ",") != "edge-station-1,edge-station-2" {
		t.Fatalf("readiness containers = %v", containers)
	}
	if err := waitForWirelessAuthentication(context.Background(), deployment, readinessInspectorStub{ready: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationFailurePreservesCredentialAndRetryState(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	originalInspector := newWirelessReadinessInspector
	originalTimeout := wirelessAuthenticationTimeout
	originalPoll := wirelessAuthenticationPoll
	ready := false
	newWirelessReadinessInspector = func() wirelessReadinessInspector { return readinessInspectorStub{ready: ready} }
	wirelessAuthenticationTimeout = 10 * time.Millisecond
	wirelessAuthenticationPoll = time.Millisecond
	t.Cleanup(func() {
		newWirelessReadinessInspector = originalInspector
		wirelessAuthenticationTimeout = originalTimeout
		wirelessAuthenticationPoll = originalPoll
	})
	t.Setenv("WIFI_PASSWORD", "authentication-passphrase")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), "example/station", "ghcr.io/gdcs-dev/wireless-client", 1))
	if err := os.WriteFile(manifestPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "wireless authentication readiness timed out") {
		t.Fatalf("authentication failure = %v", err)
	}
	if _, err := os.Stat(secrets.WirelessCredentialPath(stateRoot, "edge", "home")); err != nil {
		t.Fatalf("protected credential not retained: %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingCredentialRecreate("edge")
	if err != nil || strings.Join(pending, ",") != "gateway,station" {
		t.Fatalf("pending recreation = %v, error = %v", pending, err)
	}
	if snapshot, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists || !strings.Contains(string(snapshot), "passphraseSecretRef: wifi") {
		t.Fatalf("desired snapshot retained = %t, error = %v", exists, err)
	}
	if radios, err := store.ListWirelessRadios("edge"); err != nil || len(radios) != 3 {
		t.Fatalf("radio ownership = %#v, error = %v", radios, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	ready = true
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if pending, err := store.PendingCredentialRecreate("edge"); err != nil || len(pending) != 0 {
		t.Fatalf("pending recreation after retry = %v, error = %v", pending, err)
	}
	if _, err := os.Stat(filepath.Dir(secrets.WirelessCredentialPath(stateRoot, "edge", "home"))); err != nil {
		t.Fatal(err)
	}
}

func TestVAPFailurePreservesWirelessStateForRetry(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	stubCredentialRuntime(t, &credentialComposeRunner{})
	originalInspector := newWirelessReadinessInspector
	originalTimeout, originalPoll := wirelessVAPTimeout, wirelessVAPPoll
	ready := false
	newWirelessReadinessInspector = func() wirelessReadinessInspector {
		if !ready {
			return readinessInspectorStub{ready: true, vapError: errors.New("hostapd not ready")}
		}
		return readinessInspectorStub{ready: true}
	}
	wirelessVAPTimeout, wirelessVAPPoll = 10*time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		newWirelessReadinessInspector = originalInspector
		wirelessVAPTimeout, wirelessVAPPoll = originalTimeout, originalPoll
	})
	t.Setenv("WIFI_PASSWORD", "vap-retry-passphrase")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")
	_, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "wireless VAP readiness timed out") {
		t.Fatalf("VAP failure = %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("snapshot after failure: exists=%t err=%v", exists, err)
	}
	if radios, err := store.ListWirelessRadios("edge"); err != nil || len(radios) != 3 {
		t.Fatalf("radio ownership after failure = %#v, %v", radios, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secrets.WirelessCredentialPath(stateRoot, "edge", "home")); err != nil {
		t.Fatalf("credential after failure: %v", err)
	}
	ready = true
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if radios, err := store.ListWirelessRadios("edge"); err != nil || len(radios) != 3 {
		t.Fatalf("radio ownership after retry = %#v, %v", radios, err)
	}
}

var _ wirelessReadinessInspector = readinessInspectorStub{}

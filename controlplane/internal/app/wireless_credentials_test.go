package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/compose"
	"github.com/gdcs-dev/vcpe/controlplane/internal/hwsim"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

func TestPrepareWirelessCredentialsClassifiesConsumers(t *testing.T) {
	stateRoot := t.TempDir()
	desired := credentialDocument("wifi", manifest.WirelessWPA2Personal)
	deployment := credentialPlan()
	values := map[string]string{"wifi": "same-passphrase", "next": "same-passphrase"}

	first, err := prepareWirelessCredentials(stateRoot, manifest.Document{}, false, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, first.ForceRecreate, "gateway", "station")

	unchanged, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, unchanged.ForceRecreate)

	values["wifi"] = "rotated-passphrase"
	rotated, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, rotated.ForceRecreate, "gateway", "station")

	refChanged := credentialDocument("next", manifest.WirelessWPA2Personal)
	values["next"] = "rotated-passphrase"
	identicalNewRef, err := prepareWirelessCredentials(stateRoot, desired, true, refChanged, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, identicalNewRef.ForceRecreate, "gateway", "station")

	values["next"] = "different-passphrase"
	differentNewRef, err := prepareWirelessCredentials(stateRoot, desired, true, refChanged, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, differentNewRef.ForceRecreate, "gateway", "station")

	if err := os.Remove(secrets.WirelessCredentialPath(stateRoot, "edge", "home")); err != nil {
		t.Fatal(err)
	}
	missing, err := prepareWirelessCredentials(stateRoot, refChanged, true, refChanged, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, missing.ForceRecreate, "gateway", "station")

	wpa3 := credentialDocument("next", manifest.WirelessWPA3Personal)
	policyChanged, err := prepareWirelessCredentials(stateRoot, refChanged, true, wpa3, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, policyChanged.ForceRecreate, "gateway", "station")
}

func TestPrepareWirelessCredentialsRotatesBothMeshPeers(t *testing.T) {
	stateRoot := t.TempDir()
	mesh := &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"}
	desired := manifest.Document{Metadata: manifest.Metadata{Name: "edge"}, Spec: manifest.Spec{Services: []manifest.Service{
		{Name: "root", Radios: []manifest.Radio{{Name: "backhaul", Mode: manifest.RadioModeMesh, Mesh: mesh}}},
		{Name: "remote", Radios: []manifest.Radio{{Name: "backhaul", Mode: manifest.RadioModeMesh, Mesh: mesh}}},
	}}}
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{
		{Name: "root", Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "backhaul", Mode: manifest.RadioModeMesh, Mesh: mesh}}}}},
		{Name: "remote", Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "backhaul", Mode: manifest.RadioModeMesh, Mesh: mesh}}}}},
	}}
	values := map[string]string{"mesh-key": "first-mesh-password"}
	first, err := prepareWirelessCredentials(stateRoot, manifest.Document{}, false, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, first.ForceRecreate, "root", "remote")
	if len(first.CreatedMeshIDs) != 1 || first.CreatedMeshIDs[0] != "mesh-home" {
		t.Fatalf("created mesh files = %v", first.CreatedMeshIDs)
	}
	if data, err := os.ReadFile(secrets.MeshCredentialPath(stateRoot, "edge", "mesh-home")); err != nil || string(data) != values["mesh-key"] {
		t.Fatalf("mesh credential materialization = %q, %v", data, err)
	}
	unchanged, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, unchanged.ForceRecreate)
	values["mesh-key"] = "rotated-mesh-password"
	rotated, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, rotated.ForceRecreate, "root", "remote")
}

func TestWirelessCredentialFilesIncludesProtectedMeshMount(t *testing.T) {
	mesh := &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"}
	dep := plan.Deployment{Name: "edge"}
	service := plan.Service{Name: "root", Instances: []plan.Instance{{Radios: []plan.Radio{{Mode: manifest.RadioModeMesh, Mesh: mesh}}}}}
	files := wirelessCredentialFiles("/private/state", dep, service)
	handle, exists := files["mesh:mesh-home"]
	if !exists || handle.HostPath != secrets.MeshCredentialPath("/private/state", "edge", "mesh-home") || handle.ContainerPath != secrets.MeshCredentialContainerPath("mesh-home") {
		t.Fatalf("mesh credential mount = %+v", files)
	}
}

func TestPrepareWirelessCredentialsMirroredVAPConsumers(t *testing.T) {
	stateRoot := t.TempDir()
	desired := manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{WirelessNetworks: []manifest.WirelessNetwork{
			{Name: "home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"},
			{Name: "guest", Security: manifest.WirelessWPA2Personal, PassphraseSecretRef: "guest-ref"},
		}},
	}
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{
		{Name: "gateway", Instances: []plan.Instance{{Radios: []plan.Radio{
			{Medium: "rf24", VAPs: []plan.VAP{{Slot: 0, Network: "home"}, {Slot: 1, Network: "guest"}}},
			{Medium: "rf5", VAPs: []plan.VAP{{Slot: 0, Network: "home"}}},
		}}}},
		{Name: "station24", Instances: []plan.Instance{{Radios: []plan.Radio{{Medium: "rf24", Network: "home"}}}}},
		{Name: "station5", Instances: []plan.Instance{{Radios: []plan.Radio{{Medium: "rf5", Network: "home"}}}}},
		{Name: "guest-station", Instances: []plan.Instance{{Radios: []plan.Radio{{Medium: "rf24", Network: "guest"}}}}},
	}}
	values := map[string]string{"home-ref": "home-passphrase", "guest-ref": "guest-passphrase"}
	first, err := prepareWirelessCredentials(stateRoot, manifest.Document{}, false, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, first.ForceRecreate, "gateway", "station24", "station5", "guest-station")
	if len(first.CreatedNetworks) != 2 {
		t.Fatalf("each profile must materialize once: %v", first.CreatedNetworks)
	}
	unchanged, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, unchanged.ForceRecreate)
	values["home-ref"] = "rotated-home-passphrase"
	rotated, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, rotated.ForceRecreate, "gateway", "station24", "station5")
}

func TestMirroredCredentialRotationPreservesRadioOwnership(t *testing.T) {
	stateRoot := t.TempDir()
	desired := manifest.Document{Metadata: manifest.Metadata{Name: "edge"}, Spec: manifest.Spec{
		WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-ref"}, {Name: "guest", Security: manifest.WirelessOpen}},
	}}
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{{Name: "gateway", Instances: []plan.Instance{{
		Index: 0, ContainerName: "edge-gateway-1",
		Radios: []plan.Radio{
			{Name: "ap24", Mode: manifest.RadioModeAP, Device: "wlan0", ManagerName: "vcpe-ap24", MAC: "02:00:00:00:00:10", VAPs: []plan.VAP{{Slot: 0, Device: "wlan0", MAC: "02:00:00:00:00:10", Network: "home"}, {Slot: 7, Device: "wlan0v7", MAC: "02:00:00:00:00:17", Network: "guest"}}},
			{Name: "ap5", Mode: manifest.RadioModeAP, Device: "wlan1", ManagerName: "vcpe-ap5", MAC: "02:00:00:00:01:10", VAPs: []plan.VAP{{Slot: 0, Device: "wlan1", MAC: "02:00:00:00:01:10", Network: "home"}}},
			{Name: "ap6", Mode: manifest.RadioModeAP, Device: "wlan2", ManagerName: "vcpe-ap6", MAC: "02:00:00:00:02:10", VAPs: []plan.VAP{{Slot: 0, Device: "wlan2", MAC: "02:00:00:00:02:10", Network: "home"}}},
		},
	}}}}}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	before := append([]plan.Radio(nil), deployment.Services[0].Instances[0].Radios...)
	ownedBefore, err := store.ListWirelessRadios("edge")
	if err != nil {
		t.Fatal(err)
	}
	groupBefore, exists, err := store.WirelessGroup("edge")
	if err != nil || !exists {
		t.Fatalf("wireless group before rotation: exists=%t error=%v", exists, err)
	}
	values := map[string]string{"home-ref": "original-passphrase"}
	if _, err := prepareWirelessCredentials(stateRoot, manifest.Document{}, false, desired, deployment, values); err != nil {
		t.Fatal(err)
	}
	values["home-ref"] = "rotated-passphrase"
	changes, err := prepareWirelessCredentials(stateRoot, desired, true, desired, deployment, values)
	if err != nil {
		t.Fatal(err)
	}
	assertForcedServices(t, changes.ForceRecreate, "gateway")
	if !reflect.DeepEqual(deployment.Services[0].Instances[0].Radios, before) {
		t.Fatal("credential rotation changed radio or VAP identity")
	}
	ownedAfter, err := store.ListWirelessRadios("edge")
	if err != nil || !reflect.DeepEqual(ownedAfter, ownedBefore) {
		t.Fatalf("credential rotation changed manager ownership: before=%v after=%v error=%v", ownedBefore, ownedAfter, err)
	}
	groupAfter, exists, err := store.WirelessGroup("edge")
	if err != nil || !exists || groupAfter != groupBefore {
		t.Fatalf("credential rotation changed group: before=%v after=%v exists=%t error=%v", groupBefore, groupAfter, exists, err)
	}
}

func TestPrepareWirelessCredentialsRecreatesChangedVAPPolicy(t *testing.T) {
	base := func() (manifest.Document, plan.Deployment) {
		doc := manifest.Document{
			Metadata: manifest.Metadata{Name: "edge"},
			Spec: manifest.Spec{
				WirelessNetworks: []manifest.WirelessNetwork{
					{Name: "home", SSID: "Home", Security: manifest.WirelessOpen},
					{Name: "guest", SSID: "Guest", Security: manifest.WirelessOpen},
					{Name: "iot", SSID: "IoT", Security: manifest.WirelessOpen},
				},
				Services: []manifest.Service{
					{Name: "gateway", Radios: []manifest.Radio{{Name: "ap", Medium: "rf5", Mode: manifest.RadioModeAP,
						VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan"}, {Slot: 7, Network: "guest", Bridge: "brguest"}}}}},
					{Name: "station", Radios: []manifest.Radio{{Name: "sta", Medium: "rf5", Mode: manifest.RadioModeStation, Network: "home"}}},
				},
			},
		}
		dep := plan.Deployment{Name: "edge", Services: []plan.Service{
			{Name: "gateway", Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "ap", Medium: "rf5", Mode: manifest.RadioModeAP,
				VAPs: []plan.VAP{{Slot: 0, Network: doc.Spec.Services[0].Radios[0].VAPs[0].Network}, {Slot: 7, Network: doc.Spec.Services[0].Radios[0].VAPs[1].Network}}}}}}},
			{Name: "station", Instances: []plan.Instance{{Radios: []plan.Radio{{Name: "sta", Medium: "rf5", Mode: manifest.RadioModeStation, Network: doc.Spec.Services[1].Radios[0].Network}}}}},
		}}
		return doc, dep
	}
	for _, test := range []struct {
		name string
		edit func(*manifest.Document, *plan.Deployment)
		want []string
	}{
		{"unchanged", func(*manifest.Document, *plan.Deployment) {}, nil},
		{"reordered VAPs", func(doc *manifest.Document, _ *plan.Deployment) {
			vaps := doc.Spec.Services[0].Radios[0].VAPs
			vaps[0], vaps[1] = vaps[1], vaps[0]
		}, nil},
		{"VAP profile", func(doc *manifest.Document, dep *plan.Deployment) {
			doc.Spec.Services[0].Radios[0].VAPs[0].Network = "iot"
			dep.Services[0].Instances[0].Radios[0].VAPs[0].Network = "iot"
		}, []string{"gateway"}},
		{"VAP count", func(doc *manifest.Document, dep *plan.Deployment) {
			doc.Spec.Services[0].Radios[0].VAPs = append(doc.Spec.Services[0].Radios[0].VAPs, manifest.VAP{Slot: 3, Network: "iot", Bridge: "brlan"})
			dep.Services[0].Instances[0].Radios[0].VAPs = append(dep.Services[0].Instances[0].Radios[0].VAPs, plan.VAP{Slot: 3, Network: "iot"})
		}, []string{"gateway"}},
		{"VAP bridge", func(doc *manifest.Document, _ *plan.Deployment) {
			doc.Spec.Services[0].Radios[0].VAPs[1].Bridge = "brnew"
		}, []string{"gateway"}},
		{"station profile", func(doc *manifest.Document, dep *plan.Deployment) {
			doc.Spec.Services[1].Radios[0].Network = "guest"
			dep.Services[1].Instances[0].Radios[0].Network = "guest"
		}, []string{"station"}},
		{"profile SSID", func(doc *manifest.Document, _ *plan.Deployment) {
			doc.Spec.WirelessNetworks[0].SSID = "New Home"
		}, []string{"gateway", "station"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous, _ := base()
			desired, deployment := base()
			test.edit(&desired, &deployment)
			changes, err := prepareWirelessCredentials(t.TempDir(), previous, true, desired, deployment, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertForcedServices(t, changes.ForceRecreate, test.want...)
		})
	}
}

func credentialDocument(ref, security string) manifest.Document {
	return manifest.Document{
		Metadata: manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{WirelessNetworks: []manifest.WirelessNetwork{{
			Name: "home", Security: security, PassphraseSecretRef: ref,
		}}},
	}
}

func credentialPlan() plan.Deployment {
	return plan.Deployment{Name: "edge", Services: []plan.Service{
		{Name: "gateway", Instances: []plan.Instance{{Index: 0, Radios: []plan.Radio{{Network: "home"}}}}},
		{Name: "station", Instances: []plan.Instance{{Index: 0, Radios: []plan.Radio{{Network: "home"}}}, {Index: 1, Radios: []plan.Radio{{Network: "home"}}}}},
		{Name: "unrelated", Instances: []plan.Instance{{Index: 0}}},
	}}
}

func assertForcedServices(t *testing.T, actual map[string]bool, expected ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, service := range expected {
		want[service] = true
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("forced services = %v, want %v", actual, want)
	}
}

type credentialComposeRunner struct {
	upRequests   []compose.Request
	downRequests []compose.Request
	failService  string
	failError    error
	failDown     bool
}

func (runner *credentialComposeRunner) Up(_ context.Context, request compose.Request) (compose.OperationRecord, error) {
	runner.upRequests = append(runner.upRequests, request)
	if runner.failService != "" && strings.HasSuffix(request.ProjectName, "-"+runner.failService) {
		runner.failService = ""
		if runner.failError != nil {
			return compose.OperationRecord{}, runner.failError
		}
		return compose.OperationRecord{}, errors.New("credential sentinel startup failure")
	}
	return compose.OperationRecord{}, nil
}

func (runner *credentialComposeRunner) Down(_ context.Context, request compose.Request) (compose.OperationRecord, error) {
	runner.downRequests = append(runner.downRequests, request)
	if runner.failDown {
		return compose.OperationRecord{}, errors.New("consumer stop failed")
	}
	return compose.OperationRecord{}, nil
}

func TestCredentialRotationApplyMatrixPreservesRadioIdentity(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	t.Setenv("WIFI_PASSWORD", "first-passphrase")

	apply := func(ref, key string) []compose.Request {
		t.Helper()
		before := len(runner.upRequests)
		manifestPath := writeCredentialManifest(t, ref, key)
		if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
			t.Fatal(err)
		}
		return runner.upRequests[before:]
	}

	first := apply("wifi", "WIFI_PASSWORD")
	assertCredentialRequests(t, first, true)
	beforeRadios := persistedRadios(t, stateRoot)

	unchanged := apply("wifi", "WIFI_PASSWORD")
	assertCredentialRequests(t, unchanged, false)

	t.Setenv("WIFI_PASSWORD", "rotated-passphrase")
	rotated := apply("wifi", "WIFI_PASSWORD")
	assertCredentialRequests(t, rotated, true)

	t.Setenv("WIFI_NEXT", "rotated-passphrase")
	identicalNewRef := apply("wifi-next", "WIFI_NEXT")
	assertCredentialRequests(t, identicalNewRef, true)

	t.Setenv("WIFI_NEXT", "different-passphrase")
	differentNewRef := apply("wifi-next", "WIFI_NEXT")
	assertCredentialRequests(t, differentNewRef, true)

	credentialPath := secrets.WirelessCredentialPath(stateRoot, "edge", "home")
	if err := os.Remove(credentialPath); err != nil {
		t.Fatal(err)
	}
	repaired := apply("wifi-next", "WIFI_NEXT")
	assertCredentialRequests(t, repaired, true)
	afterRadios := persistedRadios(t, stateRoot)
	if !reflect.DeepEqual(beforeRadios, afterRadios) {
		t.Fatalf("radio identity changed across rotations:\nbefore=%#v\nafter=%#v", beforeRadios, afterRadios)
	}

	openManifest := writeCredentialManifest(t, "wifi-next", "WIFI_NEXT")
	openContents, err := os.ReadFile(openManifest)
	if err != nil {
		t.Fatal(err)
	}
	openContents = []byte(strings.Replace(string(openContents), "security: wpa2-personal, passphraseSecretRef: wifi-next", "security: open", 1))
	if err := os.WriteFile(openManifest, openContents, 0o644); err != nil {
		t.Fatal(err)
	}
	beforeOpen := len(runner.upRequests)
	if _, err := runApply(Options{Command: "apply", ManifestPath: openManifest, StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	assertCredentialRequests(t, runner.upRequests[beforeOpen:], true)
	if _, err := os.Stat(credentialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete personal credential remains after open apply: %v", err)
	}
}

func TestMirroredCredentialRotationRecreatesGatewayOnce(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	newWirelessReadinessInspector = func() wirelessReadinessInspector {
		return readinessInspectorStub{ready: true, vapStatus: []byte(fmt.Sprintf(
			"{\"radio\":\"ap\",\"slot\":0,\"interface\":\"wlan0\",\"bssid\":%q,\"enabled\":true,\"bridge\":\"brlan0\"}\n{\"radio\":\"ap5\",\"slot\":0,\"interface\":\"wlan1\",\"bssid\":%q,\"enabled\":true,\"bridge\":\"brlan0\"}\n",
			plan.CanonicalVAPBSSID("edge", "gateway", 0, "ap", 0), plan.CanonicalVAPBSSID("edge", "gateway", 0, "ap5", 0)))}
	}
	t.Setenv("WIFI_PASSWORD", "first-passphrase")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data),
		"wirelessMedia: [{name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}]",
		"wirelessMedia: [{name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}, {name: rf5, band: 5ghz, channel: 36, widthMHz: 20}]", 1))
	data = []byte(strings.Replace(string(data),
		"      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]",
		"      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}, {name: ap5, medium: rf5, device: wlan1, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]", 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	apply := func() []compose.Request {
		t.Helper()
		before := len(runner.upRequests)
		if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
			t.Fatal(err)
		}
		return runner.upRequests[before:]
	}
	assertCredentialRequests(t, apply(), true)
	before := persistedRadios(t, stateRoot)
	assertCredentialRequests(t, apply(), false)
	t.Setenv("WIFI_PASSWORD", "rotated-passphrase")
	assertCredentialRequests(t, apply(), true)
	if after := persistedRadios(t, stateRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("mirrored rotation changed manager identities: before=%#v after=%#v", before, after)
	}
}

func TestCredentialRotationFailureRetryAndDownRecovery(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	t.Setenv("WIFI_PASSWORD", "first-passphrase")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("WIFI_PASSWORD", "rotated-passphrase")
	runner.failService = "station"
	beforeFailure := len(runner.upRequests)
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "startup failure") {
		t.Fatalf("rotation failure = %v", err)
	}
	failedRequests := runner.upRequests[beforeFailure:]
	if len(failedRequests) != 2 || !failedRequests[0].ForceRecreate || !failedRequests[1].ForceRecreate {
		t.Fatalf("partial rotation requests = %#v", failedRequests)
	}
	credentialPath := secrets.WirelessCredentialPath(stateRoot, "edge", "home")
	credential, err := os.ReadFile(credentialPath)
	if err != nil || string(credential) != "rotated-passphrase" {
		t.Fatalf("retained credential = %q, error = %v", credential, err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, exists, err := store.LatestDesiredSnapshot("edge")
	if err != nil || !exists || !strings.Contains(string(snapshot), "passphraseSecretRef: wifi") || strings.Contains(string(snapshot), "rotated-passphrase") {
		t.Fatalf("retained desired snapshot = %q, exists=%t, error=%v", snapshot, exists, err)
	}
	pending, err := store.PendingCredentialRecreate("edge")
	if err != nil || strings.Join(pending, ",") != "gateway,station" {
		t.Fatalf("pending recreation = %v, error=%v", pending, err)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 3 {
		t.Fatalf("retained radio ownership = %#v, error=%v", radios, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	beforeRetry := len(runner.upRequests)
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	assertCredentialRequests(t, runner.upRequests[beforeRetry:], true)

	runner.failDown = true
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err == nil {
		t.Fatal("down succeeded despite consumer stop failure")
	}
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("credential removed while consumer may be running: %v", err)
	}
	runner.failDown = false
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err != nil {
		t.Fatalf("retry down: %v", err)
	}
	if _, err := os.Stat(credentialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential remains after down: %v", err)
	}
}

func TestCredentialPersonalModesAndDeploymentIsolation(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	t.Setenv("EDGE_A_WIFI", "edge-a-passphrase")
	t.Setenv("EDGE_B_WIFI", "edge-b-passphrase")

	for _, test := range []struct {
		name     string
		security string
		key      string
	}{
		{name: "edge-a", security: manifest.WirelessWPA2Personal, key: "EDGE_A_WIFI"},
		{name: "edge-b", security: manifest.WirelessWPA3Personal, key: "EDGE_B_WIFI"},
	} {
		manifestPath := writeCredentialManifestForDeployment(t, test.name, test.security, "wifi", test.key)
		if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
			t.Fatalf("apply %s: %v", test.name, err)
		}
	}

	for _, test := range []struct {
		name string
		want string
	}{
		{name: "edge-a", want: "edge-a-passphrase"},
		{name: "edge-b", want: "edge-b-passphrase"},
	} {
		contents, err := os.ReadFile(secrets.WirelessCredentialPath(stateRoot, test.name, "home"))
		if err != nil || string(contents) != test.want {
			t.Fatalf("credential %s = %q, error = %v", test.name, contents, err)
		}
	}
	if _, err := runDown(Options{Command: "down", Name: "edge-a", StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secrets.WirelessCredentialPath(stateRoot, "edge-a", "home")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("edge-a credential remains after down: %v", err)
	}
	if contents, err := os.ReadFile(secrets.WirelessCredentialPath(stateRoot, "edge-b", "home")); err != nil || string(contents) != "edge-b-passphrase" {
		t.Fatalf("edge-b credential changed during edge-a down: %q, error = %v", contents, err)
	}
}

func TestCredentialInvalidValueRejectedBeforeRuntime(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	t.Setenv("WIFI_PASSWORD", "short")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")
	_, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "8 through 63 printable ASCII") || strings.Contains(err.Error(), "short") {
		t.Fatalf("invalid credential error = %v", err)
	}
	if len(runner.upRequests) != 0 {
		t.Fatalf("runtime was invoked for invalid credential: %#v", runner.upRequests)
	}
	if _, err := os.Stat(secrets.WirelessCredentialPath(stateRoot, "edge", "home")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid credential was materialized: %v", err)
	}
}

func TestCredentialSentinelAbsentFromObservableSurfaces(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	sentinel := "SENTINEL-wireless-passphrase"
	runner := &credentialComposeRunner{
		failService: "station",
		failError:   fmt.Errorf("runtime echoed %s", sentinel),
	}
	stubCredentialRuntime(t, runner)
	t.Setenv("WIFI_PASSWORD", sentinel)
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")

	stderr, applyErr := captureStderr(t, func() error {
		_, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
		return err
	})
	if applyErr == nil {
		t.Fatal("apply unexpectedly succeeded")
	}
	for surface, contents := range map[string]string{
		"apply error":    applyErr.Error(),
		"structured log": stderr,
	} {
		assertNoSentinel(t, surface, contents, sentinel)
	}

	credential, err := os.ReadFile(secrets.WirelessCredentialPath(stateRoot, "edge", "home"))
	if err != nil || string(credential) != sentinel {
		t.Fatalf("protected credential = %q, error = %v", credential, err)
	}
	credentialRoot := secrets.RuntimeCredentialsRoot(stateRoot)
	if err := filepath.WalkDir(stateRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == credentialRoot {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertNoSentinel(t, path, string(contents), sentinel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	status, err := runStatus(Options{Name: "edge", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	logs, err := runLogs(Options{Name: "edge", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := json.Marshal(runner.upRequests)
	if err != nil {
		t.Fatal(err)
	}
	for surface, contents := range map[string]string{
		"status":            status.Message,
		"logs":              logs.Message,
		"compose arguments": string(requests),
	} {
		assertNoSentinel(t, surface, contents, sentinel)
	}
}

func TestCredentialCreatedBeforeRuntimeIsRemovedOnRollback(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	sentinel := "rollback-passphrase"
	t.Setenv("WIFI_PASSWORD", sentinel)
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_FAIL_PHASE", "credentials")
	manifestPath := writeCredentialManifest(t, "wifi", "WIFI_PASSWORD")

	_, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("rollback error = %v", err)
	}
	credentialPath := secrets.WirelessCredentialPath(stateRoot, "edge", "home")
	if _, err := os.Stat(credentialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new credential remains after pre-runtime rollback: %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || exists {
		t.Fatalf("snapshot after rollback: exists=%t error=%v", exists, err)
	}
}

func captureStderr(t *testing.T, action func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	actionErr := action()
	_ = writer.Close()
	os.Stderr = original
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(output), actionErr
}

func assertNoSentinel(t *testing.T, surface, contents, sentinel string) {
	t.Helper()
	if strings.Contains(contents, sentinel) {
		t.Fatalf("credential sentinel leaked through %s", surface)
	}
}

func assertCredentialRequests(t *testing.T, requests []compose.Request, forceConsumers bool) {
	t.Helper()
	if len(requests) != 3 {
		t.Fatalf("compose requests = %#v", requests)
	}
	for _, request := range requests {
		service := strings.TrimPrefix(request.ProjectName, "edge-")
		wantForce := forceConsumers && (service == "gateway" || service == "station")
		if request.ForceRecreate != wantForce {
			t.Errorf("%s ForceRecreate = %t, want %t", service, request.ForceRecreate, wantForce)
		}
		if wantForce && service == "station" && strings.Join(request.Services, ",") != "station-1,station-2" {
			t.Errorf("station services = %v", request.Services)
		}
	}
}

func persistedRadios(t *testing.T, stateRoot string) []persist.WirelessRadio {
	t.Helper()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	radios, err := store.ListWirelessRadios("edge")
	if err != nil {
		t.Fatal(err)
	}
	return radios
}

func stubCredentialRuntime(t *testing.T, runner *credentialComposeRunner) {
	t.Helper()
	originalCompose := newComposeRunner
	originalNetwork := newNetworkProvisioner
	originalInspector := newContainerPIDInspector
	originalPreflightClient := newHWSIMClient
	originalManager := newWirelessManager
	originalReadiness := newWirelessReadinessInspector
	newComposeRunner = func() composeLifecycleRunner { return runner }
	newNetworkProvisioner = func() networkProvisioner { return &recordingNetworkProvisioner{} }
	newContainerPIDInspector = func() hwsim.ContainerPIDInspector { return fixedPIDInspector(501) }
	newHWSIMClient = func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(healthyHWSIMTransport{}), nil
	}
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) {
		return wirelessManagerStub{
			ensure: func(_ context.Context, _ hwsim.ContainerPIDInspector, _ string, _ hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
				return hwsim.EnsureResult{}, nil
			},
			release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
				return hwsim.ReleaseResult{Name: name, Released: true}, nil
			},
			gc: func(context.Context, []string, bool) (hwsim.GCResult, error) { return hwsim.GCResult{}, nil },
		}, nil
	}
	newWirelessReadinessInspector = func() wirelessReadinessInspector { return readinessInspectorStub{ready: true} }
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_REPO_ROOT", t.TempDir())
	t.Cleanup(func() {
		newComposeRunner = originalCompose
		newNetworkProvisioner = originalNetwork
		newContainerPIDInspector = originalInspector
		newHWSIMClient = originalPreflightClient
		newWirelessManager = originalManager
		newWirelessReadinessInspector = originalReadiness
	})
}

func writeCredentialManifest(t *testing.T, ref, key string) string {
	t.Helper()
	return writeCredentialManifestForDeployment(t, "edge", manifest.WirelessWPA2Personal, ref, key)
}

func writeCredentialManifestForDeployment(t *testing.T, deployment, security, ref, key string) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), "wireless-personal.yaml")
	cidr := "10.10.10.0/24"
	if deployment == "edge-b" {
		cidr = "10.10.20.0/24"
	}
	contents := fmt.Sprintf(`apiVersion: vcpe.dev/v1
kind: Deployment
metadata: {name: %s}
spec:
  networks:
    - {role: mgmt, ipv4: {cidr: %s}}
  wirelessMedia: [{name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}]
`+"  wirelessNetworks: [{name: home, ssid: vcpe-lab, security: %s, passphraseSecretRef: %s}]\n"+`  secrets:
    - {name: %s, provider: env, key: %s}
  services:
    - name: gateway
      type: generic-container
      replicas: 1
      image: {repository: example/gateway, tag: test}
      interfaces: [{role: mgmt}]
      bridges: [{name: brlan0}]
      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]
    - name: station
      type: generic-container
      replicas: 2
      image: {repository: example/station, tag: test}
      interfaces: [{role: mgmt}]
      radios: [{name: sta, medium: rf24, network: home, device: wlan0, mode: station}]
    - name: unrelated
      type: generic-container
      replicas: 1
      image: {repository: example/unrelated, tag: test}
      interfaces: [{role: mgmt}]
`, deployment, cidr, security, ref, ref, key)
	if err := os.WriteFile(manifestPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifestPath
}

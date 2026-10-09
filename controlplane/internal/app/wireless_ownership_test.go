package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/compose"
	"github.com/gdcs-dev/vcpe/controlplane/internal/hwsim"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

type wirelessEnsurerFunc func(context.Context, hwsim.ContainerPIDInspector, string, hwsim.EnsureRequest) (hwsim.EnsureResult, error)

func (fn wirelessEnsurerFunc) EnsureForContainer(ctx context.Context, inspector hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
	return fn(ctx, inspector, container, request)
}

func (fn wirelessEnsurerFunc) Release(context.Context, string) (hwsim.ReleaseResult, error) {
	return hwsim.ReleaseResult{Released: true}, nil
}

func (fn wirelessEnsurerFunc) List(context.Context) ([]hwsim.ListEntry, error) {
	return nil, nil
}

func (fn wirelessEnsurerFunc) GC(context.Context, []string, bool) (hwsim.GCResult, error) {
	return hwsim.GCResult{}, nil
}

type wirelessManagerStub struct {
	ensure  wirelessEnsurerFunc
	list    func(context.Context) ([]hwsim.ListEntry, error)
	release func(context.Context, string) (hwsim.ReleaseResult, error)
	gc      func(context.Context, []string, bool) (hwsim.GCResult, error)
}

func (stub wirelessManagerStub) EnsureForContainer(ctx context.Context, inspector hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
	return stub.ensure(ctx, inspector, container, request)
}

func (stub wirelessManagerStub) List(ctx context.Context) ([]hwsim.ListEntry, error) {
	if stub.list == nil {
		return nil, nil
	}
	return stub.list(ctx)
}

func (stub wirelessManagerStub) Release(ctx context.Context, name string) (hwsim.ReleaseResult, error) {
	return stub.release(ctx, name)
}

func (stub wirelessManagerStub) GC(ctx context.Context, keep []string, allowEmpty bool) (hwsim.GCResult, error) {
	if stub.gc == nil {
		return hwsim.GCResult{}, nil
	}
	return stub.gc(ctx, keep, allowEmpty)
}

type fixedPIDInspector int

func (pid fixedPIDInspector) ContainerPID(context.Context, string) (int, error) {
	return int(pid), nil
}

type orderedComposeRunner struct{ events *[]string }

func (runner orderedComposeRunner) Up(context.Context, compose.Request) (compose.OperationRecord, error) {
	*runner.events = append(*runner.events, "compose-up")
	return compose.OperationRecord{}, nil
}

func (runner orderedComposeRunner) Down(context.Context, compose.Request) (compose.OperationRecord, error) {
	*runner.events = append(*runner.events, "compose-down")
	return compose.OperationRecord{}, nil
}

type orderedNetworkProvisioner struct{ events *[]string }

func (provisioner orderedNetworkProvisioner) EnsureNetwork(context.Context, podman.NetworkSpec) error {
	return nil
}

func (provisioner orderedNetworkProvisioner) RemoveNetwork(context.Context, string) error {
	*provisioner.events = append(*provisioner.events, "networks")
	return nil
}

func TestPrepareWirelessOwnershipAssignsAndRollsBackNewGroup(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()

	rollback, obsolete, err := prepareWirelessOwnership(store, &deployment)
	if err != nil {
		t.Fatal(err)
	}
	if len(obsolete) != 0 {
		t.Fatalf("obsolete = %#v, want empty", obsolete)
	}
	if got := deployment.Services[0].Instances[0].Radios[0].GroupMask; got != 1 {
		t.Fatalf("group mask = %#x, want 0x1", got)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 1 {
		t.Fatalf("radios = %#v, error = %v", radios, err)
	}
	if radios[0].ManagerName != "vcpe-radio" || radios[0].ContainerName != "edge-station-1" || radios[0].Status != "planned" {
		t.Fatalf("persisted radio = %#v", radios[0])
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || exists {
		t.Fatalf("group after rollback: exists=%t error=%v", exists, err)
	}
	if radios, err := store.ListWirelessRadios("edge"); err != nil || len(radios) != 0 {
		t.Fatalf("radios after rollback = %#v, error = %v", radios, err)
	}
}

func TestPrepareWirelessOwnershipRestoresPriorRecordsForReusedGroup(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	group, err := store.AllocateWirelessGroup("edge")
	if err != nil {
		t.Fatal(err)
	}
	previous := persist.WirelessRadio{
		Deployment: "edge", Service: "station", Replica: 0, LogicalName: "old",
		ManagerName: "vcpe-old", GroupBit: group.Bit, Status: "ready",
	}
	if err := store.ReplaceWirelessRadios("edge", []persist.WirelessRadio{previous}); err != nil {
		t.Fatal(err)
	}
	deployment := wirelessOwnershipPlan()
	rollback, _, err := prepareWirelessOwnership(store, &deployment)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 1 || radios[0].ManagerName != "vcpe-old" || radios[0].Status != "ready" {
		t.Fatalf("restored radios = %#v, error = %v", radios, err)
	}
	reused, exists, err := store.WirelessGroup("edge")
	if err != nil || !exists || reused.Bit != group.Bit {
		t.Fatalf("reused group = %#v, exists=%t error=%v", reused, exists, err)
	}
}

func TestPrepareWirelessOwnershipPersistsMeshBridgeWithAccessGroup(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()
	instance := &deployment.Services[0].Instances[0]
	instance.Radios[0].Mode = manifest.RadioModeAP
	instance.Radios[0].Name = "access"
	instance.Radios = append(instance.Radios, plan.Radio{
		Name: "backhaul", Device: "mesh0", Mode: manifest.RadioModeMesh, Bridge: "brlan",
		Mesh:        &manifest.Mesh{ID: "mesh-home", Bridge: "brlan", SAESecretRef: "mesh-key"},
		ManagerName: "vcpe-mesh", MAC: "02:00:00:00:00:02", ContainerName: "edge-station-1",
	})
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 2 {
		t.Fatalf("persisted radios = %+v, error = %v", radios, err)
	}
	if radios[0].Bridge != "" || radios[1].Bridge != "brlan" || radios[0].GroupBit != radios[1].GroupBit || instance.Radios[0].GroupMask != instance.Radios[1].GroupMask {
		t.Fatalf("mesh bridge or shared group lost: %+v", radios)
	}
}

func wirelessOwnershipPlan() plan.Deployment {
	return plan.Deployment{
		Name: "edge",
		Services: []plan.Service{{
			Name: "station",
			Instances: []plan.Instance{{
				Index: 0, InstanceName: "station-1", ContainerName: "edge-station-1",
				Radios: []plan.Radio{{
					Name: "wifi", Network: "home", Device: "wlan0", Mode: "station",
					ManagerName: "vcpe-radio", MAC: "02:00:00:00:00:01", ContainerName: "edge-station-1",
				}},
			}},
		}},
	}
}

func TestTriBandVAPsRemainOutsideManagerOwnership(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := plan.Deployment{Name: "edge", Services: []plan.Service{{Name: "gateway", Instances: []plan.Instance{{
		Index: 0, ContainerName: "edge-gateway-1",
	}}}}}
	instance := &deployment.Services[0].Instances[0]
	for radioIndex := 0; radioIndex < 3; radioIndex++ {
		radio := plan.Radio{
			Name: fmt.Sprintf("ap%d", radioIndex), Device: fmt.Sprintf("wlan%d", radioIndex),
			ManagerName: fmt.Sprintf("vcpe-ap-%d", radioIndex), MAC: fmt.Sprintf("02:00:00:00:%02x:10", radioIndex),
			Mode: "ap", ContainerName: "edge-gateway-1",
		}
		for slot := 0; slot < 8; slot++ {
			vap := plan.VAP{
				Slot: slot, Device: fmt.Sprintf("wlan%dv%d", radioIndex, slot),
				MAC: fmt.Sprintf("02:00:00:00:%02x:%02x", radioIndex, slot+16),
			}
			if slot == 0 {
				vap.Device, vap.MAC = radio.Device, radio.MAC
			}
			radio.VAPs = append(radio.VAPs, vap)
		}
		instance.Radios = append(instance.Radios, radio)
	}
	if _, obsolete, err := prepareWirelessOwnership(store, &deployment); err != nil || len(obsolete) != 0 {
		t.Fatalf("prepare ownership: obsolete=%v error=%v", obsolete, err)
	}
	owned, err := store.ListWirelessRadios("edge")
	if err != nil || len(owned) != 3 {
		t.Fatalf("manager records = %v, error = %v", owned, err)
	}
	var ensured []string
	manager := wirelessEnsurerFunc(func(_ context.Context, _ hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
		if container != "edge-gateway-1" || request.GroupMask != 1 {
			t.Fatalf("container/request = %q %#v", container, request)
		}
		ensured = append(ensured, request.Name)
		return hwsim.EnsureResult{}, nil
	})
	if err := reconcileWireless(context.Background(), store, deployment, nil, manager, fixedPIDInspector(42)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ensured, ","); got != "vcpe-ap-0,vcpe-ap-1,vcpe-ap-2" {
		t.Fatalf("manager ensures = %q", got)
	}
	for _, radio := range owned {
		if radio.Device != "wlan"+strings.TrimPrefix(radio.LogicalName, "ap") {
			t.Fatalf("secondary BSS entered manager ownership: %#v", radio)
		}
	}
}

func TestTriBand24VAPApplyConvergesWithThreeGatewayPHYs(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	runner := &credentialComposeRunner{}
	stubCredentialRuntime(t, runner)
	ensuredGateway := []string{}
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) {
		return wirelessManagerStub{
			ensure: func(_ context.Context, _ hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
				if container == "wireless-gateway-1" {
					ensuredGateway = append(ensuredGateway, request.Name)
				}
				return hwsim.EnsureResult{}, nil
			},
			release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
				return hwsim.ReleaseResult{Name: name, Released: true}, nil
			},
			gc: func(context.Context, []string, bool) (hwsim.GCResult, error) { return hwsim.GCResult{}, nil },
		}, nil
	}
	var status strings.Builder
	for _, radio := range []struct {
		name, device string
	}{{"ap24", "wlan0"}, {"ap5", "wlan1"}, {"ap6", "wlan2"}} {
		for slot := 0; slot < 8; slot++ {
			device := radio.device
			if slot > 0 {
				device = plan.VAPDeviceName("wireless", "gateway", 0, radio.name, slot)
			}
			bridge := []string{"brlan0", "brguest", "briot", "brhotspot", "brbackhaul", "brlan0", "brguest", "briot"}[slot]
			fmt.Fprintf(&status, "{\"radio\":%q,\"slot\":%d,\"interface\":%q,\"bssid\":%q,\"enabled\":true,\"bridge\":%q}\n",
				radio.name, slot, device, plan.CanonicalVAPBSSID("wireless", "gateway", 0, radio.name, slot), bridge)
		}
	}
	vapStatus := status.String()
	newWirelessReadinessInspector = func() wirelessReadinessInspector {
		return readinessInspectorStub{ready: true, vapStatus: []byte(vapStatus)}
	}
	manifestPath := filepath.Join("..", "..", "..", "manifests", "dev", "wireless.yaml")
	apply := func() {
		t.Helper()
		if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	before := persistedRadiosForDeployment(t, stateRoot, "wireless")
	if len(before) != 6 {
		t.Fatalf("manager-owned PHYs = %d, want three Gateway and three station PHYs", len(before))
	}
	response, err := runStatus(Options{Name: "wireless", StateRoot: stateRoot, OutputJSON: true})
	if err != nil || strings.Count(response.Message, `"state": "ready"`) != 24 {
		t.Fatalf("24 VAP status count = %d, error = %v", strings.Count(response.Message, `"state": "ready"`), err)
	}
	apply()
	if after := persistedRadiosForDeployment(t, stateRoot, "wireless"); !reflect.DeepEqual(before, after) {
		t.Fatalf("unchanged reapply changed manager identities: before=%#v after=%#v", before, after)
	}
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	modified := strings.Replace(string(original), "{slot: 7, network: lab-c, bridge: briot}", "{slot: 7, network: lab-c, bridge: brlan0}", 1)
	manifestPath = filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(manifestPath, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}
	vapStatus = strings.Replace(vapStatus,
		fmt.Sprintf("\"radio\":\"ap24\",\"slot\":7,\"interface\":%q,\"bssid\":%q,\"enabled\":true,\"bridge\":\"briot\"",
			plan.VAPDeviceName("wireless", "gateway", 0, "ap24", 7), plan.CanonicalVAPBSSID("wireless", "gateway", 0, "ap24", 7)),
		fmt.Sprintf("\"radio\":\"ap24\",\"slot\":7,\"interface\":%q,\"bssid\":%q,\"enabled\":true,\"bridge\":\"brlan0\"",
			plan.VAPDeviceName("wireless", "gateway", 0, "ap24", 7), plan.CanonicalVAPBSSID("wireless", "gateway", 0, "ap24", 7)), 1)
	beforeEnsures, beforeCompose := len(ensuredGateway), len(runner.upRequests)
	apply()
	if got := ensuredGateway[beforeEnsures:]; len(got) != 3 || len(map[string]bool{got[0]: true, got[1]: true, got[2]: true}) != 3 {
		t.Fatalf("Gateway recreation ensured %v, want three distinct PHYs", got)
	}
	forceGateway := 0
	for _, request := range runner.upRequests[beforeCompose:] {
		if request.ProjectName == "wireless-gateway" && request.ForceRecreate {
			forceGateway++
		}
	}
	if forceGateway != 1 {
		t.Fatalf("Gateway recreations = %d, want one", forceGateway)
	}
	if after := persistedRadiosForDeployment(t, stateRoot, "wireless"); !reflect.DeepEqual(before, after) {
		t.Fatalf("VAP policy change replaced PHY ownership: before=%#v after=%#v", before, after)
	}
	disruptivePath := filepath.Join(t.TempDir(), "medium-change.yaml")
	disruptive := strings.Replace(modified, "{name: rf5, band: 5ghz, channel: 36, widthMHz: 20}", "{name: rf5, band: 5ghz, channel: 40, widthMHz: 20}", 1)
	if err := os.WriteFile(disruptivePath, []byte(disruptive), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runApply(Options{Command: "apply", ManifestPath: disruptivePath, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "disruptive") {
		t.Fatalf("unapproved medium change = %v", err)
	}
	response, err = runStatus(Options{Name: "wireless", StateRoot: stateRoot, OutputJSON: true})
	if err != nil || strings.Count(response.Message, `"state": "ready"`) != 24 {
		t.Fatalf("VAPs after Gateway recreation: ready=%d err=%v", strings.Count(response.Message, `"state": "ready"`), err)
	}
	readyStatus := vapStatus
	vapStatus = strings.Replace(vapStatus, `"radio":"ap6","slot":7`, `"radio":"ap6-failed","slot":7`, 1)
	originalTimeout, originalPoll := wirelessVAPTimeout, wirelessVAPPoll
	wirelessVAPTimeout, wirelessVAPPoll = 10*time.Millisecond, time.Millisecond
	t.Cleanup(func() { wirelessVAPTimeout, wirelessVAPPoll = originalTimeout, originalPoll })
	if _, err := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "wireless VAP readiness timed out") {
		t.Fatalf("one-BSS failure = %v", err)
	}
	if after := persistedRadiosForDeployment(t, stateRoot, "wireless"); !reflect.DeepEqual(before, after) {
		t.Fatalf("one-BSS failure replaced PHY ownership: before=%#v after=%#v", before, after)
	}
	vapStatus = readyStatus
	apply()
	if _, err := runDown(Options{Command: "down", Name: "wireless", StateRoot: stateRoot}); err != nil {
		t.Fatalf("down after recovery: %v", err)
	}
	if radios := persistedRadiosForDeployment(t, stateRoot, "wireless"); len(radios) != 0 {
		t.Fatalf("manager records after down: %#v", radios)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.WirelessGroup("wireless"); err != nil || exists {
		t.Fatalf("wireless group after down: exists=%t err=%v", exists, err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("wireless"); err != nil || exists {
		t.Fatalf("desired snapshot after down: exists=%t err=%v", exists, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secrets.WirelessCredentialPath(stateRoot, "wireless", "home")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("protected credential after down: %v", err)
	}
}

func persistedRadiosForDeployment(t *testing.T, stateRoot, deployment string) []persist.WirelessRadio {
	t.Helper()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	radios, err := store.ListWirelessRadios(deployment)
	if err != nil {
		t.Fatal(err)
	}
	return radios
}

func TestReconcileWirelessUsesDeterministicOrderAndRecordsReady(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()
	deployment.Services[0].Instances[0].Radios = append(deployment.Services[0].Instances[0].Radios,
		plan.Radio{Name: "wifi-z", Network: "home", Device: "wlan1", Mode: "station", ManagerName: "vcpe-z", MAC: "02:00:00:00:00:02", ContainerName: "edge-station-1"},
	)
	deployment.Services[0].Instances[0].Radios[0].ManagerName = "vcpe-a"
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	var names []string
	manager := wirelessEnsurerFunc(func(_ context.Context, _ hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
		if container != "edge-station-1" || request.GroupMask != 1 {
			t.Fatalf("container/request = %q %#v", container, request)
		}
		names = append(names, request.Name)
		return hwsim.EnsureResult{}, nil
	})
	if err := reconcileWireless(context.Background(), store, deployment, nil, manager, fixedPIDInspector(42)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "vcpe-a,vcpe-z" {
		t.Fatalf("ensure order = %v", names)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil {
		t.Fatal(err)
	}
	for _, radio := range radios {
		if radio.Status != "ready" {
			t.Fatalf("radio status = %#v", radio)
		}
	}
}

func TestMeshReapplyRepairsPeersIntoRecreatedContainers(t *testing.T) {
	doc, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meshNames := map[string]bool{}
	meshByPID := map[int]map[string]bool{}
	manager := wirelessEnsurerFunc(func(ctx context.Context, inspector hwsim.ContainerPIDInspector, container string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
		if request.RadioType == manifest.RadioModeMesh {
			pid, err := inspector.ContainerPID(ctx, container)
			if err != nil {
				return hwsim.EnsureResult{}, err
			}
			if meshByPID[pid] == nil {
				meshByPID[pid] = map[string]bool{}
			}
			meshByPID[pid][request.Name] = true
			meshNames[request.Name] = true
			if request.GroupMask != 1 || request.InterfaceName != "mesh0" {
				t.Fatalf("mesh repair request = %+v", request)
			}
		}
		return hwsim.EnsureResult{}, nil
	})
	for _, pid := range []fixedPIDInspector{42, 84} {
		if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
			t.Fatal(err)
		}
		if err := reconcileWireless(context.Background(), store, deployment, nil, manager, pid); err != nil {
			t.Fatal(err)
		}
		if err := waitForGatewayMeshes(context.Background(), deployment, readinessInspectorStub{meshStatus: []byte(`{"radio":"backhaul","interface":"mesh0","bridge":"brlan0","joined":true,"peers":1}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if len(meshNames) != 2 || len(meshByPID[42]) != 2 || len(meshByPID[84]) != 2 {
		t.Fatalf("mesh re-ensures after recreation = %v, want two stable identities at both PIDs", meshByPID)
	}
}

func TestReconcileWirelessRecordsPartialFailure(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	manager := wirelessEnsurerFunc(func(context.Context, hwsim.ContainerPIDInspector, string, hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
		return hwsim.EnsureResult{}, errors.New("namespace unavailable")
	})
	err = reconcileWireless(context.Background(), store, deployment, nil, manager, fixedPIDInspector(42))
	if err == nil || !strings.Contains(err.Error(), "namespace unavailable") {
		t.Fatalf("error = %v", err)
	}
	radios, listErr := store.ListWirelessRadios("edge")
	if listErr != nil || len(radios) != 1 || radios[0].Status != "failed: namespace unavailable" {
		t.Fatalf("radios = %#v, error = %v", radios, listErr)
	}
}

func TestReconcileWirelessReleasesChangedAndRemovedRadiosBeforeEnsure(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	group, err := store.AllocateWirelessGroup("edge")
	if err != nil {
		t.Fatal(err)
	}
	previous := []persist.WirelessRadio{
		{Deployment: "edge", Service: "station", Replica: 0, LogicalName: "wifi", ManagerName: "vcpe-radio", MAC: "02:00:00:00:00:01", Network: "home", Device: "old0", Mode: "station", ContainerName: "edge-station-1", GroupBit: group.Bit, Status: "ready"},
		{Deployment: "edge", Service: "station", Replica: 1, LogicalName: "wifi", ManagerName: "vcpe-removed", MAC: "02:00:00:00:00:02", Network: "home", Device: "wlan0", Mode: "station", ContainerName: "edge-station-2", GroupBit: group.Bit, Status: "ready"},
	}
	if err := store.ReplaceWirelessRadios("edge", previous); err != nil {
		t.Fatal(err)
	}
	deployment := wirelessOwnershipPlan()
	_, obsolete, err := prepareWirelessOwnership(store, &deployment)
	if err != nil {
		t.Fatal(err)
	}
	if len(obsolete) != 2 {
		t.Fatalf("obsolete = %#v", obsolete)
	}
	var events []string
	manager := wirelessManagerStub{
		ensure: func(_ context.Context, _ hwsim.ContainerPIDInspector, _ string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
			events = append(events, "ensure:"+request.Name)
			return hwsim.EnsureResult{}, nil
		},
		release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			events = append(events, "release:"+name)
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		},
	}
	if err := reconcileWireless(context.Background(), store, deployment, obsolete, manager, fixedPIDInspector(42)); err != nil {
		t.Fatal(err)
	}
	want := "release:vcpe-radio,release:vcpe-removed,ensure:vcpe-radio"
	if strings.Join(events, ",") != want {
		t.Fatalf("events = %v, want %s", events, want)
	}
}

func TestReconcileWirelessReleasesReturnedRadioBeforeEnsure(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	var events []string
	manager := wirelessManagerStub{
		list: func(context.Context) ([]hwsim.ListEntry, error) {
			return []hwsim.ListEntry{
				{Record: hwsim.Radio{Name: "vcpe-radio", Lifecycle: "returned"}},
				{Record: hwsim.Radio{Name: "vcpe-neighbor", Lifecycle: "returned"}},
			}, nil
		},
		ensure: func(_ context.Context, _ hwsim.ContainerPIDInspector, _ string, request hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
			events = append(events, "ensure:"+request.Name)
			return hwsim.EnsureResult{}, nil
		},
		release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			events = append(events, "release:"+name)
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		},
	}
	if err := reconcileWireless(context.Background(), store, deployment, nil, manager, fixedPIDInspector(42)); err != nil {
		t.Fatal(err)
	}
	if want := "release:vcpe-radio,ensure:vcpe-radio"; strings.Join(events, ",") != want {
		t.Fatalf("events = %v, want %s", events, want)
	}
}

func TestReconcileWirelessScaleToZeroReleasesRadioAndGroup(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	deployment := wirelessOwnershipPlan()
	if _, _, err := prepareWirelessOwnership(store, &deployment); err != nil {
		t.Fatal(err)
	}
	empty := plan.Deployment{Name: "edge"}
	_, obsolete, err := prepareWirelessOwnership(store, &empty)
	if err != nil {
		t.Fatal(err)
	}
	var released []string
	manager := wirelessManagerStub{
		ensure: func(context.Context, hwsim.ContainerPIDInspector, string, hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
			t.Fatal("ensure called for scale-to-zero")
			return hwsim.EnsureResult{}, nil
		},
		release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			released = append(released, name)
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		},
	}
	if err := reconcileWireless(context.Background(), store, empty, obsolete, manager, fixedPIDInspector(42)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(released, ",") != "vcpe-radio" {
		t.Fatalf("released = %v", released)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || exists {
		t.Fatalf("group after scale-to-zero: exists=%t error=%v", exists, err)
	}
}

func TestGarbageCollectWirelessUsesAllActiveDeployments(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var want []string
	for _, deployment := range []string{"edge-b", "edge-a"} {
		snapshot := wirelessSnapshot(deployment)
		if err := store.SaveDesiredSnapshot(deployment, []byte(snapshot)); err != nil {
			t.Fatal(err)
		}
		group, err := store.AllocateWirelessGroup(deployment)
		if err != nil {
			t.Fatal(err)
		}
		managerName := plan.RadioManagerName(deployment, "gateway", 0, "ap")
		want = append(want, managerName)
		if err := store.ReplaceWirelessRadios(deployment, []persist.WirelessRadio{{
			Deployment: deployment, Service: "gateway", Replica: 0, LogicalName: "ap",
			ManagerName: managerName, GroupBit: group.Bit, Status: "ready",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(want)
	called := false
	manager := wirelessManagerStub{gc: func(_ context.Context, keep []string, allowEmpty bool) (hwsim.GCResult, error) {
		called = true
		if allowEmpty || !reflect.DeepEqual(keep, want) {
			t.Fatalf("gc keep = %v, allowEmpty = %t, want %v", keep, allowEmpty, want)
		}
		return hwsim.GCResult{Preserved: keep}, nil
	}}
	if err := garbageCollectWireless(context.Background(), store, manager); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("manager GC was not called")
	}
}

func TestGarbageCollectWirelessSkipsManagerForIncompleteView(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveDesiredSnapshot("broken", []byte("not: [valid")); err != nil {
		t.Fatal(err)
	}
	called := false
	manager := wirelessManagerStub{gc: func(context.Context, []string, bool) (hwsim.GCResult, error) {
		called = true
		return hwsim.GCResult{}, nil
	}}
	err = garbageCollectWireless(context.Background(), store, manager)
	if err == nil || !strings.Contains(err.Error(), "decode active deployment broken") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("manager GC called with incomplete global view")
	}
}

func TestRFBaselinePlansKeepActiveDeploymentsAndRejectIncompleteView(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveDesiredSnapshot("edge-a", []byte(wirelessSnapshot("edge-a"))); err != nil {
		t.Fatal(err)
	}
	group, err := store.AllocateWirelessGroup("edge-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceWirelessRadios("edge-a", []persist.WirelessRadio{{
		Deployment: "edge-a", Service: "gateway", LogicalName: "ap", ManagerName: plan.RadioManagerName("edge-a", "gateway", 0, "ap"), GroupBit: group.Bit,
	}}); err != nil {
		t.Fatal(err)
	}
	incoming := plan.Deployment{Name: "edge-b", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:00:00:02"}}}}}}}
	plans, err := rfBaselinePlans(store, incoming)
	if err != nil || len(plans) != 2 || plans[0].Name != "edge-a" || plans[1].Name != "edge-b" {
		t.Fatalf("baseline plans = %+v, error = %v", plans, err)
	}
	if err := store.SaveDesiredSnapshot("broken", []byte("not: [valid")); err != nil {
		t.Fatal(err)
	}
	if _, err := rfBaselinePlans(store, incoming); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("incomplete active baseline = %v", err)
	}
	if err := store.DeleteDeploymentSnapshot("broken"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceWirelessRadios("edge-a", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rfBaselinePlans(store, incoming); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("missing active radio ownership = %v", err)
	}
}

func TestDownWaitsForGlobalWriterLock(t *testing.T) {
	stateRoot := t.TempDir()
	lock, err := state.AcquireWriterLock(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := runDown(Options{Name: "missing", StateRoot: stateRoot})
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("down bypassed the writer lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "unknown deployment") {
			t.Fatalf("down after lock release = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("down stayed blocked after writer lock release")
	}
}

func TestStateResetWaitsForGlobalWriterLock(t *testing.T) {
	stateRoot := t.TempDir()
	previousManager := newWirelessManager
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) {
		return nil, errors.New("manager reached")
	}
	t.Cleanup(func() { newWirelessManager = previousManager })
	lock, err := state.AcquireWriterLock(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := runState(Options{CommandArgs: []string{"reset"}, StateRoot: stateRoot})
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("state reset bypassed the writer lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("state reset stayed blocked after writer lock release")
	}
}

func TestRFBaselineAfterDownRetainsOtherActiveDeployment(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, name := range []string{"edge-a", "edge-b"} {
		if err := store.SaveDesiredSnapshot(name, []byte(wirelessSnapshot(name))); err != nil {
			t.Fatal(err)
		}
		group, err := store.AllocateWirelessGroup(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.ReplaceWirelessRadios(name, []persist.WirelessRadio{{
			Deployment: name, Service: "gateway", LogicalName: "ap", ManagerName: plan.RadioManagerName(name, "gateway", 0, "ap"), GroupBit: group.Bit,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	plans, err := rfBaselinePlansAfterDown(store, "edge-a")
	if err != nil || len(plans) != 1 || plans[0].Name != "edge-b" {
		t.Fatalf("RF keep plans = %+v, error = %v", plans, err)
	}
	if _, ok, err := store.LatestDesiredSnapshot("edge-a"); err != nil || !ok {
		t.Fatalf("excluded snapshot lost before teardown: %v, exists = %t", err, ok)
	}
}

type rfBaselineControllerStub struct {
	install func(context.Context, string) error
	clear   func(context.Context) error
}

func (stub rfBaselineControllerStub) InstallBaseline(ctx context.Context, config string) error {
	return stub.install(ctx, config)
}

func (stub rfBaselineControllerStub) ClearBaseline(ctx context.Context) error {
	return stub.clear(ctx)
}

func TestReconcileRFAfterDownKeepsOtherDeploymentsOrClearsLast(t *testing.T) {
	var actions []string
	installed := true
	var commands []string
	client := func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(rfHealthTransport{installed: &installed, commands: &commands}), nil
	}
	controller := rfBaselineControllerStub{
		install: func(_ context.Context, config string) error {
			actions = append(actions, "install")
			if !strings.Contains(config, "(0,1,35)") {
				t.Fatalf("surviving deployment lost RF link: %s", config)
			}
			return nil
		},
		clear: func(context.Context) error { actions = append(actions, "clear"); return nil },
	}
	survivor := plan.Deployment{Name: "other", WirelessScenarios: []plan.WirelessScenario{{Name: "roam"}}, Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:00:00:01"}, {MAC: "02:00:00:00:00:02"}}}}}}}
	if err := reconcileRFAfterDown(context.Background(), []plan.Deployment{survivor}, true, controller, client); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actions, []string{"install"}) || !strings.Contains(strings.Join(commands, ","), "medium-health") {
		t.Fatalf("surviving RF reconciliation = %v, health = %v", actions, commands)
	}
	actions = nil
	if err := reconcileRFAfterDown(context.Background(), nil, true, controller, client); err != nil || !reflect.DeepEqual(actions, []string{"clear"}) {
		t.Fatalf("last RF cleanup = %v, actions = %v", err, actions)
	}
	actions = nil
	if err := reconcileRFAfterDown(context.Background(), nil, false, controller, client); err != nil || len(actions) != 0 {
		t.Fatalf("legacy teardown touched RF = %v, actions = %v", err, actions)
	}
}

func TestDownKeepsRFOwnershipOnHostFailureForRetry(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDesiredSnapshot("edge", []byte(wirelessLifecycleSnapshot("edge")+"  wirelessScenarios: [{name: roam}]\n")); err != nil {
		t.Fatal(err)
	}
	store.Close()
	var events []string
	stubWirelessDownAdapters(t, &events, wirelessManagerStub{release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
		return hwsim.ReleaseResult{Name: name, Released: true}, nil
	}})
	previousController := newRFBaselineInstaller
	clearFails := true
	newRFBaselineInstaller = func() rfBaselineController {
		return rfBaselineControllerStub{
			install: func(context.Context, string) error { t.Fatal("unexpected RF install after last down"); return nil },
			clear: func(context.Context) error {
				if clearFails {
					return errors.New("host RF unavailable")
				}
				return nil
			},
		}
	}
	t.Cleanup(func() { newRFBaselineInstaller = previousController })
	options := Options{Command: "down", Name: "edge", StateRoot: stateRoot}
	if _, err := runDown(options); err == nil || !strings.Contains(err.Error(), "host RF unavailable") {
		t.Fatalf("down RF failure = %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("snapshot lost after RF failure = %t, error = %v", exists, err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || !exists {
		t.Fatalf("wireless group lost after RF failure = %t, error = %v", exists, err)
	}
	store.Close()
	clearFails = false
	if _, err := runDown(options); err != nil {
		t.Fatalf("retry down after RF recovery: %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || exists {
		t.Fatalf("snapshot remains after RF cleanup = %t, error = %v", exists, err)
	}
}

func wirelessSnapshot(deployment string) string {
	return fmt.Sprintf("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: %s}\nspec:\n  wirelessMedia:\n    - {name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}\n  wirelessNetworks:\n    - {name: home, ssid: vcpe-lab, security: open}\n  services:\n    - name: gateway\n      type: generic-container\n      replicas: 1\n      image: {repository: example/gateway, tag: test}\n      bridges: [{name: brlan0}]\n      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]\n", deployment)
}

type healthyHWSIMTransport struct{}

func (healthyHWSIMTransport) Execute(_ context.Context, args ...string) (hwsim.CommandResult, error) {
	var data string
	switch args[0] {
	case "version":
		data = `{"version":"test"}`
	case "list":
		data = `[]`
	case "doctor":
		data = `{"healthy":true,"checks":[]}`
	default:
		return hwsim.CommandResult{}, fmt.Errorf("unexpected preflight command %q", args[0])
	}
	return hwsim.CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"` + args[0] + `","success":true,"data":` + data + `}`)}, nil
}

func TestWirelessApplyDownApplyOrdersPhasesAndRecordsSuccess(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "wireless.yaml")
	if err := os.WriteFile(manifestPath, []byte(wirelessLifecycleSnapshot("edge")), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []string
	manager := wirelessManagerStub{
		ensure: func(ctx context.Context, inspector hwsim.ContainerPIDInspector, _ string, _ hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
			pid, err := inspector.ContainerPID(ctx, "edge-gateway-1")
			if err != nil || pid != 501 {
				t.Fatalf("container PID = %d, error = %v", pid, err)
			}
			events = append(events, "ensure")
			return hwsim.EnsureResult{}, nil
		},
		release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			events = append(events, "release")
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		},
		gc: func(context.Context, []string, bool) (hwsim.GCResult, error) {
			events = append(events, "gc")
			return hwsim.GCResult{}, nil
		},
	}
	stubWirelessApplyAdapters(t, &events, manager)
	applyOptions := Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}
	if _, err := runApply(applyOptions); err != nil {
		t.Fatal(err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := store.RecentOperations(1)
	if err != nil || len(operations) != 1 || operations[0].Status != "succeeded" {
		t.Fatalf("operations = %#v, error = %v", operations, err)
	}
	phases, err := store.OperationPhases(operations[0].OperationID)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleIndex, wirelessIndex := -1, -1
	for index, phase := range phases {
		if phase.Phase == "lifecycle" && phase.Status == "succeeded" {
			lifecycleIndex = index
		}
		if phase.Phase == "wireless" && phase.Status == "succeeded" {
			wirelessIndex = index
		}
	}
	if lifecycleIndex < 0 || wirelessIndex <= lifecycleIndex {
		t.Fatalf("operation phases = %#v", phases)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := runApply(applyOptions); err != nil {
		t.Fatalf("unchanged reapply: %v", err)
	}
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	if _, err := runApply(applyOptions); err != nil {
		t.Fatal(err)
	}
	want := "compose-up,ensure,gc,compose-up,ensure,gc,compose-down,release,networks,compose-up,ensure,gc"
	if strings.Join(events, ",") != want {
		t.Fatalf("lifecycle events = %v, want %s", events, want)
	}
}

func TestWirelessApplyRecordsPartialEnsureFailureAndRetainsState(t *testing.T) {
	types.Register()
	stateRoot := t.TempDir()
	manifestData := strings.Replace(wirelessLifecycleSnapshot("edge"),
		"    - {name: home, ssid: vcpe-lab, security: open}",
		"    - {name: home, ssid: vcpe-lab, security: open}\n    - {name: guest, ssid: vcpe-guest, security: open}", 1)
	manifestData = strings.Replace(manifestData,
		"      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]",
		"      radios:\n        - {name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}\n        - {name: guest, medium: rf24, device: wlan1, mode: ap, vaps: [{slot: 0, network: guest, bridge: brlan0}]}", 1)
	manifestPath := filepath.Join(t.TempDir(), "wireless-partial.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []string
	ensureCalls := 0
	manager := wirelessManagerStub{
		ensure: func(context.Context, hwsim.ContainerPIDInspector, string, hwsim.EnsureRequest) (hwsim.EnsureResult, error) {
			ensureCalls++
			if ensureCalls == 2 {
				return hwsim.EnsureResult{}, errors.New("second radio unavailable")
			}
			return hwsim.EnsureResult{}, nil
		},
	}
	stubWirelessApplyAdapters(t, &events, manager)
	_, applyErr := runApply(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if applyErr == nil || !strings.Contains(applyErr.Error(), "second radio unavailable") {
		t.Fatalf("apply error = %v", applyErr)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	operations, err := store.RecentOperations(1)
	if err != nil || len(operations) != 1 || operations[0].Status != "failed" {
		t.Fatalf("operations = %#v, error = %v", operations, err)
	}
	phases, err := store.OperationPhases(operations[0].OperationID)
	if err != nil {
		t.Fatal(err)
	}
	foundWirelessFailure, foundRollbackSkip := false, false
	for _, phase := range phases {
		foundWirelessFailure = foundWirelessFailure || phase.Phase == "wireless" && phase.Status == "failed"
		foundRollbackSkip = foundRollbackSkip || phase.Phase == "rollback" && phase.Status == "skipped"
	}
	if !foundWirelessFailure || !foundRollbackSkip {
		t.Fatalf("operation phases = %#v", phases)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 2 || radios[0].Status != "ready" || !strings.HasPrefix(radios[1].Status, "failed:") {
		t.Fatalf("retained radios = %#v, error = %v", radios, err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("retained snapshot: exists=%t error=%v", exists, err)
	}
}

func TestDownOrdersContainersRadiosNetworksAndStateCleanup(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	var events []string
	manager := wirelessManagerStub{
		release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			events = append(events, "radios")
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		},
	}
	stubWirelessDownAdapters(t, &events, manager)
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "compose-down,radios,networks" {
		t.Fatalf("teardown events = %v", events)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if radios, err := store.ListWirelessRadios("edge"); err != nil || len(radios) != 0 {
		t.Fatalf("radios after down = %#v, error = %v", radios, err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || exists {
		t.Fatalf("group after down: exists=%t error=%v", exists, err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || exists {
		t.Fatalf("snapshot after down: exists=%t error=%v", exists, err)
	}
}

func TestDownRetainsWirelessStateWhenReleaseFails(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	var events []string
	manager := wirelessManagerStub{
		release: func(context.Context, string) (hwsim.ReleaseResult, error) {
			events = append(events, "radios")
			return hwsim.ReleaseResult{}, errors.New("release unavailable")
		},
	}
	stubWirelessDownAdapters(t, &events, manager)
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "release unavailable") {
		t.Fatalf("error = %v", err)
	}
	if strings.Join(events, ",") != "compose-down,radios" {
		t.Fatalf("teardown events = %v", events)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	radios, err := store.ListWirelessRadios("edge")
	if err != nil || len(radios) != 1 || !strings.HasPrefix(radios[0].Status, "release-failed:") {
		t.Fatalf("retained radios = %#v, error = %v", radios, err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || !exists {
		t.Fatalf("retained group: exists=%t error=%v", exists, err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("retained snapshot: exists=%t error=%v", exists, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) {
		return wirelessManagerStub{release: func(_ context.Context, name string) (hwsim.ReleaseResult, error) {
			events = append(events, "radios")
			return hwsim.ReleaseResult{Name: name, Released: true}, nil
		}}, nil
	}
	if _, err := runDown(Options{Command: "down", Name: "edge", StateRoot: stateRoot}); err != nil {
		t.Fatalf("retry down: %v", err)
	}
	if strings.Join(events, ",") != "compose-down,radios,compose-down,radios,networks" {
		t.Fatalf("retry teardown events = %v", events)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, exists, err := store.WirelessGroup("edge"); err != nil || exists {
		t.Fatalf("group after retry: exists=%t error=%v", exists, err)
	}
}

func TestStateResetGarbageCollectsAllManagedRadiosBeforeClearingState(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	if _, err := secrets.MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("reset-passphrase")); err != nil {
		t.Fatal(err)
	}
	garbageCollected := false
	stubWirelessManagerFactory(t, wirelessManagerStub{
		gc: func(_ context.Context, keep []string, allowEmpty bool) (hwsim.GCResult, error) {
			if len(keep) != 0 || !allowEmpty {
				t.Fatalf("GC keep=%v allowEmpty=%t", keep, allowEmpty)
			}
			garbageCollected = true
			return hwsim.GCResult{}, nil
		},
	})
	if _, err := runState(Options{CommandArgs: []string{"reset"}, StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	if !garbageCollected {
		t.Fatal("state reset did not garbage collect managed radios")
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if leases, err := store.ListIPAMLeases(); err != nil || len(leases) != 0 {
		t.Fatalf("leases after reset = %#v, error = %v", leases, err)
	}
	if _, err := os.Stat(secrets.RuntimeCredentialsRoot(stateRoot)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credentials remain after reset: %v", err)
	}
}

func TestStateResetPreservesStateWhenManagerCleanupFails(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	credentialPath := secrets.WirelessCredentialPath(stateRoot, "edge", "home")
	if _, err := secrets.MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("reset-passphrase")); err != nil {
		t.Fatal(err)
	}
	stubWirelessManagerFactory(t, wirelessManagerStub{
		gc: func(context.Context, []string, bool) (hwsim.GCResult, error) {
			return hwsim.GCResult{}, errors.New("manager cleanup unavailable")
		},
	})
	if _, err := runState(Options{CommandArgs: []string{"reset"}, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "manager cleanup unavailable") {
		t.Fatalf("error = %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if leases, err := store.ListIPAMLeases(); err != nil || len(leases) != 1 {
		t.Fatalf("retained leases = %#v, error = %v", leases, err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || !exists {
		t.Fatalf("retained group: exists=%t error=%v", exists, err)
	}
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("retained snapshot: exists=%t error=%v", exists, err)
	}
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("credential not retained after failed reset: %v", err)
	}
}

func TestStateResetPreservesRFStateWhenHostCleanupFails(t *testing.T) {
	stateRoot := seedWirelessDownState(t)
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := wirelessLifecycleSnapshot("edge") + "  wirelessScenarios: [{name: roam}]\n"
	if err := store.SaveDesiredSnapshot("edge", []byte(snapshot)); err != nil {
		t.Fatal(err)
	}
	store.Close()
	stubWirelessManagerFactory(t, wirelessManagerStub{})
	previousController := newRFBaselineInstaller
	newRFBaselineInstaller = func() rfBaselineController {
		return rfBaselineControllerStub{
			install: func(context.Context, string) error { t.Fatal("unexpected RF install during reset"); return nil },
			clear:   func(context.Context) error { return errors.New("host cleanup unavailable") },
		}
	}
	t.Cleanup(func() { newRFBaselineInstaller = previousController })
	if _, err := runState(Options{CommandArgs: []string{"reset"}, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "host cleanup unavailable") {
		t.Fatalf("RF reset cleanup error = %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, exists, err := store.LatestDesiredSnapshot("edge"); err != nil || !exists {
		t.Fatalf("RF snapshot retained after failed reset = %t, error = %v", exists, err)
	}
	if _, exists, err := store.WirelessGroup("edge"); err != nil || !exists {
		t.Fatalf("RF ownership retained after failed reset = %t, error = %v", exists, err)
	}
}

func stubWirelessManagerFactory(t *testing.T, manager wirelessRadioManager) {
	t.Helper()
	originalManager := newWirelessManager
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) { return manager, nil }
	t.Cleanup(func() { newWirelessManager = originalManager })
}

func stubWirelessApplyAdapters(t *testing.T, events *[]string, manager wirelessRadioManager) {
	t.Helper()
	originalCompose := newComposeRunner
	originalNetwork := newNetworkProvisioner
	originalInspector := newContainerPIDInspector
	originalReadinessInspector := newWirelessReadinessInspector
	originalPreflightClient := newHWSIMClient
	newComposeRunner = func() composeLifecycleRunner { return orderedComposeRunner{events: events} }
	newNetworkProvisioner = func() networkProvisioner { return orderedNetworkProvisioner{events: events} }
	newContainerPIDInspector = func() hwsim.ContainerPIDInspector { return fixedPIDInspector(501) }
	newWirelessReadinessInspector = func() wirelessReadinessInspector { return readinessInspectorStub{ready: true} }
	newHWSIMClient = func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(healthyHWSIMTransport{}), nil
	}
	stubWirelessManagerFactory(t, manager)
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_REPO_ROOT", t.TempDir())
	t.Cleanup(func() {
		newComposeRunner = originalCompose
		newNetworkProvisioner = originalNetwork
		newContainerPIDInspector = originalInspector
		newWirelessReadinessInspector = originalReadinessInspector
		newHWSIMClient = originalPreflightClient
	})
}

func stubWirelessDownAdapters(t *testing.T, events *[]string, manager wirelessRadioManager) {
	t.Helper()
	originalCompose := newComposeRunner
	originalNetwork := newNetworkProvisioner
	newComposeRunner = func() composeLifecycleRunner { return orderedComposeRunner{events: events} }
	newNetworkProvisioner = func() networkProvisioner { return orderedNetworkProvisioner{events: events} }
	stubWirelessManagerFactory(t, manager)
	t.Cleanup(func() {
		newComposeRunner = originalCompose
		newNetworkProvisioner = originalNetwork
	})
}

func seedWirelessDownState(t *testing.T) string {
	t.Helper()
	stateRoot := t.TempDir()
	snapshot := wirelessLifecycleSnapshot("edge")
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDesiredSnapshot("edge", []byte(snapshot)); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceCustomerLeases("edge", []persist.IPAMLease{{CustomerID: "edge", Role: "mgmt", CIDR: "10.10.10.0/24"}}); err != nil {
		t.Fatal(err)
	}
	group, err := store.AllocateWirelessGroup("edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceWirelessRadios("edge", []persist.WirelessRadio{{
		Deployment: "edge", Service: "gateway", Replica: 0, LogicalName: "ap",
		ManagerName: plan.RadioManagerName("edge", "gateway", 0, "ap"), GroupBit: group.Bit, Status: "ready",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	serviceDirectory := filepath.Join(stateRoot, "artifacts", "v1", "deployments", "edge", "runtime", "gateway")
	if err := os.MkdirAll(serviceDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDirectory, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDirectory, "compose.env"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VCPE_REPO_ROOT", t.TempDir())
	return stateRoot
}

func wirelessLifecycleSnapshot(deployment string) string {
	return fmt.Sprintf("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: %s}\nspec:\n  networks:\n    - {role: mgmt, ipv4: {cidr: 10.10.10.0/24}}\n  wirelessMedia:\n    - {name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}\n  wirelessNetworks:\n    - {name: home, ssid: vcpe-lab, security: open}\n  services:\n    - name: gateway\n      type: generic-container\n      replicas: 1\n      image: {repository: example/gateway, tag: test}\n      interfaces: [{role: mgmt}]\n      bridges: [{name: brlan0}]\n      radios: [{name: ap, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}]\n", deployment)
}

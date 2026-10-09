package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/rfmedium"
)

type scenarioFixture struct {
	sync.Mutex
	initial, final string
	bssid          string
	steps          []int
	stepTimes      []time.Time
	restores       int
	healthError    error
	probeError     error
	noRoam         bool
	loseProbes     bool
	changeIPv4     bool
	badIPv4        bool
	failStep       bool
	baselines      []string
	updatedNames   []string
	linkResets     []string
	fallbacks      int
	failRestore    bool
}

func (fixture *scenarioFixture) InstallBaseline(_ context.Context, baseline string) error {
	fixture.Lock()
	defer fixture.Unlock()
	fixture.baselines = append(fixture.baselines, baseline)
	return nil
}

func (fixture *scenarioFixture) RestoreBaseline(context.Context, string) error {
	fixture.Lock()
	defer fixture.Unlock()
	fixture.fallbacks++
	return nil
}

func (fixture *scenarioFixture) RestoreScenarioLinks(_ context.Context, deployment plan.Deployment, name string) error {
	fixture.Lock()
	defer fixture.Unlock()
	fixture.restores++
	fixture.linkResets = append(fixture.linkResets, deployment.Name+"/"+name)
	if fixture.failRestore && fixture.restores == 2 {
		return errors.New("link reset failed")
	}
	return nil
}

func (fixture *scenarioFixture) CheckHealthy(context.Context) error {
	fixture.Lock()
	defer fixture.Unlock()
	return fixture.healthError
}

func (fixture *scenarioFixture) UpdateScenarioStep(_ context.Context, deployment plan.Deployment, _ string, index int) error {
	fixture.Lock()
	defer fixture.Unlock()
	fixture.steps = append(fixture.steps, index)
	fixture.updatedNames = append(fixture.updatedNames, deployment.Name)
	fixture.stepTimes = append(fixture.stepTimes, time.Now())
	if index == 2 {
		if fixture.failStep {
			return errors.New("RF daemon update failed")
		}
		fixture.probeError = nil
		if fixture.loseProbes {
			fixture.probeError = errors.New("LAN unavailable")
		}
		fixture.badIPv4 = fixture.changeIPv4
	}
	if index == 3 && !fixture.noRoam {
		fixture.bssid = fixture.final
	}
	return nil
}

type scenarioInspectorStub struct {
	*scenarioFixture
	deployment plan.Deployment
}

func (stub scenarioInspectorStub) ContainerVAPStatus(_ context.Context, container string) ([]byte, error) {
	var output strings.Builder
	for _, service := range stub.deployment.Services {
		for _, instance := range service.Instances {
			if instance.PodmanContainerName(stub.deployment.Name, service.Name) != container {
				continue
			}
			for _, radio := range instance.Radios {
				for _, vap := range radio.VAPs {
					fmt.Fprintf(&output, "{\"radio\":%q,\"slot\":%d,\"interface\":%q,\"bssid\":%q,\"enabled\":true,\"bridge\":%q}\n", radio.Name, vap.Slot, vap.Device, vap.MAC, vap.Bridge)
				}
			}
		}
	}
	return []byte(output.String()), nil
}

func (stub scenarioInspectorStub) ContainerMeshStatus(context.Context, string) ([]byte, error) {
	return []byte(`{"radio":"backhaul","interface":"mesh0","bridge":"brlan0","joined":true,"peers":1}`), nil
}

func (stub scenarioInspectorStub) ContainerFile(_ context.Context, container, _ string) ([]byte, error) {
	if strings.Contains(container, "remote-probe") {
		return []byte("radio=wifi\ndevice=wlan0\nbssid=" + stub.final + "\nipv4=10.0.0.100/24\nstate=ready\n"), nil
	}
	return []byte("state=ready\n"), nil
}

func (stub scenarioInspectorStub) ProbeLAN(context.Context, string, string, string) error {
	stub.Lock()
	defer stub.Unlock()
	return stub.probeError
}

func (stub scenarioInspectorStub) ObserveStation(context.Context, string, string) (podman.StationObservation, error) {
	stub.Lock()
	defer stub.Unlock()
	ipv4 := netip.MustParsePrefix("10.0.0.101/24")
	if stub.badIPv4 {
		ipv4 = netip.MustParsePrefix("10.0.0.102/24")
	}
	return podman.StationObservation{Authenticated: true, BSSID: stub.bssid, IPv4: ipv4}, nil
}

func newScenarioFixture(t *testing.T) (plan.Deployment, plan.WirelessScenario, *scenarioFixture, scenarioInspectorStub) {
	t.Helper()
	document, err := manifest.Load(filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := planner.Build(document, nil)
	if err != nil {
		t.Fatal(err)
	}
	scenario := deployment.WirelessScenarios[0]
	for index := range scenario.Steps {
		if scenario.Steps[index].AtMs != 0 {
			scenario.Steps[index].AtMs = 30
		}
	}
	scenario.Assertions.MaxRoamMs = 500
	fixture := &scenarioFixture{initial: scenario.Assertions.InitialAP.BSSID, final: scenario.Assertions.FinalAP.BSSID, bssid: scenario.Assertions.InitialAP.BSSID}
	return deployment, scenario, fixture, scenarioInspectorStub{fixture, deployment}
}

func TestScenarioRunSchedulesStepsAndRestoresBaseline(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	outcome, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.initialBSSID != fixture.initial || outcome.finalBSSID != fixture.final || outcome.roam <= 0 || outcome.samples == 0 {
		t.Fatalf("incomplete scenario outcome: %+v", outcome)
	}
	fixture.Lock()
	defer fixture.Unlock()
	if fmt.Sprint(fixture.steps) != "[0 1 2 3]" || fixture.restores != 2 || fixture.fallbacks != 0 {
		t.Fatalf("steps = %v, link resets = %d, daemon restarts = %d", fixture.steps, fixture.restores, fixture.fallbacks)
	}
	if fixture.stepTimes[2].Sub(fixture.stepTimes[1]) < 20*time.Millisecond {
		t.Fatalf("timed crossover applied early: %v", fixture.stepTimes)
	}
}

func TestScenarioRunFailsOnStuckClientAndRestoresBaseline(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	scenario.Assertions.MaxRoamMs = 40
	fixture.noRoam = true
	_, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("stuck client error = %v", err)
	}
	if fixture.restores != 2 {
		t.Fatalf("restores after timeout = %d", fixture.restores)
	}
}

func TestScenarioRunTimesOutBeforeLateStep(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	fixture.noRoam = true
	scenario.Steps[3].AtMs = 200
	scenario.Assertions.MaxRoamMs = 40
	_, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("late step timeout = %v", err)
	}
	if fmt.Sprint(fixture.steps) != "[0 1 2]" || fixture.restores != 2 {
		t.Fatalf("steps = %v, restores = %d", fixture.steps, fixture.restores)
	}
}

func TestScenarioRunRestoresOnCancellationAndDaemonLoss(t *testing.T) {
	for _, test := range []struct {
		name string
		fail func(context.CancelFunc, *scenarioFixture)
	}{
		{"cancellation", func(cancel context.CancelFunc, _ *scenarioFixture) { cancel() }},
		{"daemon loss", func(_ context.CancelFunc, fixture *scenarioFixture) {
			fixture.Lock()
			fixture.healthError = errors.New("daemon lost")
			fixture.Unlock()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment, scenario, fixture, inspector := newScenarioFixture(t)
			fixture.noRoam = true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				for {
					fixture.Lock()
					stepped := len(fixture.steps) > 2
					fixture.Unlock()
					if stepped {
						test.fail(cancel, fixture)
						return
					}
					time.Sleep(time.Millisecond)
				}
			}()
			_, err := executeWirelessScenario(ctx, deployment, scenario, "global-baseline", fixture, inspector)
			if err == nil || fixture.restores != 2 {
				t.Fatalf("failure = %v, restores = %d", err, fixture.restores)
			}
		})
	}
}

func TestScenarioRunEnforcesProbeGapIPv4AndUpdateFailures(t *testing.T) {
	for _, test := range []struct {
		name, want string
		setup      func(*scenarioFixture, *plan.WirelessScenario)
	}{
		{"probe outage", "LAN probe", func(fixture *scenarioFixture, scenario *plan.WirelessScenario) {
			fixture.loseProbes = true
			scenario.Assertions.MaxGapMs = 40
		}},
		{"IPv4 change", "station IPv4 changed", func(fixture *scenarioFixture, _ *plan.WirelessScenario) { fixture.changeIPv4 = true }},
		{"RF update failure", "RF daemon update failed", func(fixture *scenarioFixture, _ *plan.WirelessScenario) { fixture.failStep = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment, scenario, fixture, inspector := newScenarioFixture(t)
			test.setup(fixture, &scenario)
			_, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
			if err == nil || !strings.Contains(err.Error(), test.want) || fixture.restores != 2 {
				t.Fatalf("scenario error = %v, restores = %d", err, fixture.restores)
			}
		})
	}
}

func TestScenarioRunRestoresGlobalUnionWithoutUpdatingNeighbor(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	neighbor := plan.Deployment{Name: "another-deployment", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:ff:00:01"}, {MAC: "02:00:00:ff:00:02"}}}}}}}
	baseline, err := rfmedium.BuildBaseline([]plan.Deployment{deployment, neighbor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executeWirelessScenario(context.Background(), deployment, scenario, baseline, fixture, inspector); err != nil {
		t.Fatal(err)
	}
	for _, installed := range fixture.baselines {
		if installed != baseline || !strings.Contains(installed, "02:00:00:ff:00:01") {
			t.Fatalf("restored baseline lost neighboring deployment: %q", installed)
		}
	}
	for _, name := range fixture.updatedNames {
		if name != deployment.Name {
			t.Fatalf("updated another deployment: %v", fixture.updatedNames)
		}
	}
	for _, name := range fixture.linkResets {
		if name != deployment.Name+"/"+scenario.Name {
			t.Fatalf("reset another deployment: %v", fixture.linkResets)
		}
	}
	if fixture.fallbacks != 0 {
		t.Fatalf("ordinary scenario restarted global daemon %d times", fixture.fallbacks)
	}
}

func TestScenarioRunRestoresAfterInitialAPTimeout(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	fixture.bssid = fixture.final
	previousTimeout, previousPoll := wirelessAuthenticationTimeout, wirelessAuthenticationPoll
	wirelessAuthenticationTimeout, wirelessAuthenticationPoll = 10*time.Millisecond, time.Millisecond
	t.Cleanup(func() { wirelessAuthenticationTimeout, wirelessAuthenticationPoll = previousTimeout, previousPoll })
	_, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
	if err == nil || !strings.Contains(err.Error(), "initial AP") || fixture.restores != 2 {
		t.Fatalf("initial AP failure = %v, restores = %d", err, fixture.restores)
	}
}

func TestScenarioRunUsesGlobalRecoveryOnlyAfterLinkResetFailure(t *testing.T) {
	deployment, scenario, fixture, inspector := newScenarioFixture(t)
	fixture.failRestore = true
	_, err := executeWirelessScenario(context.Background(), deployment, scenario, "global-baseline", fixture, inspector)
	if err == nil || !strings.Contains(err.Error(), "link reset failed") || fixture.restores != 2 || fixture.fallbacks != 1 {
		t.Fatalf("failed link reset = %v, link attempts = %d, global recoveries = %d", err, fixture.restores, fixture.fallbacks)
	}
}

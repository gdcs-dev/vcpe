package rfmedium

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/hwsim"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

type hostCommandFunc func(context.Context, []byte, ...string) (int, error)

func (run hostCommandFunc) Execute(ctx context.Context, input []byte, args ...string) (int, error) {
	return run(ctx, input, args...)
}

func TestInstallBaselineStartsOrRestartsOnlyWhenNeeded(t *testing.T) {
	for _, test := range []struct {
		name        string
		compareExit int
		want        []string
	}{
		{"unchanged", 0, []string{"install", "cmp", "rm", "start"}},
		{"changed", 1, []string{"install", "cmp", "mv", "restart"}},
		{"missing", 2, []string{"install", "cmp", "test", "mv", "restart"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var operations []string
			host := Host{Transport: hostCommandFunc(func(_ context.Context, input []byte, args ...string) (int, error) {
				operation := args[0]
				if operation == "systemctl" {
					operation = args[1]
				}
				operations = append(operations, operation)
				if args[0] == "install" && string(input) != "baseline\n" {
					t.Fatalf("installed baseline = %q", input)
				}
				if args[0] == "cmp" {
					return test.compareExit, nil
				}
				return 0, nil
			})}
			if err := host.InstallBaseline(context.Background(), "baseline\n"); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(operations, test.want) {
				t.Fatalf("operations = %v, want %v", operations, test.want)
			}
		})
	}
}

func TestRestoreBaselineRestartsUnchangedDaemonAndChecksHealth(t *testing.T) {
	var operations []string
	host := Host{Transport: hostCommandFunc(func(_ context.Context, _ []byte, args ...string) (int, error) {
		operation := args[0]
		if operation == "systemctl" {
			operation = args[1]
		}
		operations = append(operations, operation)
		return 0, nil
	})}
	if err := host.RestoreBaseline(context.Background(), "baseline\n"); err != nil {
		t.Fatal(err)
	}
	if err := host.CheckHealthy(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "cmp", "rm", "start", "restart", "is-active"}
	if !reflect.DeepEqual(operations, want) {
		t.Fatalf("host operations = %v, want %v", operations, want)
	}
}

func TestInstallBaselineFailsClosedOnUnexpectedCompareOrRestartError(t *testing.T) {
	for _, failAt := range []string{"install", "cmp", "restart"} {
		t.Run(failAt, func(t *testing.T) {
			var operations []string
			host := Host{Transport: hostCommandFunc(func(_ context.Context, _ []byte, args ...string) (int, error) {
				operation := args[0]
				if operation == "systemctl" {
					operation = args[1]
				}
				operations = append(operations, operation)
				if operation == failAt {
					if failAt == "cmp" {
						return 2, nil
					}
					return 0, fmt.Errorf("host unavailable")
				}
				if args[0] == "cmp" || args[0] == "test" {
					return 1, nil
				}
				return 0, nil
			})}
			err := host.InstallBaseline(context.Background(), "baseline\n")
			if err == nil || !strings.Contains(err.Error(), failAt) {
				t.Fatalf("install error = %v, operations = %v", err, operations)
			}
			for _, operation := range operations {
				if failAt != "restart" && operation == "restart" {
					t.Fatalf("restarted after %s failure: %v", failAt, operations)
				}
			}
		})
	}
}

func TestInstallBaselineOnSupportedMachine(t *testing.T) {
	if os.Getenv("VCPE_RF_BASELINE_LIVE") != "1" {
		t.Skip("requires an idle supported Podman Machine and VCPE_RF_BASELINE_LIVE=1")
	}
	baseline, err := BuildBaseline([]plan.Deployment{{Name: "bootstrap", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{
		{MAC: "02:00:00:00:ee:01"}, {MAC: "02:00:00:00:ee:02"},
	}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := NewHost().InstallBaseline(ctx, baseline); err != nil {
		t.Fatal(err)
	}
	transport, err := hwsim.NewCommandTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status, err := hwsim.NewClient(transport).MediumHealth(ctx)
	if err != nil || !status.Healthy {
		t.Fatalf("medium health = %+v, error = %v", status, err)
	}
	for _, required := range []string{"baseline", "service", "socket", "registration"} {
		found := false
		for _, check := range status.Checks {
			if check.Name == required && check.OK {
				found = true
			}
		}
		if !found {
			t.Errorf("medium health check %q missing: %+v", required, status.Checks)
		}
	}
	scenario := plan.WirelessScenario{
		Name: "bootstrap", Station: plan.ScenarioStation{MAC: "02:00:00:00:ee:01"},
		Steps: []plan.ScenarioStep{
			{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:02"}, SNRDb: -75},
			{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:02"}, SNRDb: 35},
		},
	}
	deployment := plan.Deployment{Name: "bootstrap", WirelessScenarios: []plan.WirelessScenario{scenario}}
	t.Cleanup(func() {
		if err := NewHost().UpdateScenarioStep(context.Background(), deployment, scenario.Name, 1); err != nil {
			t.Errorf("restore host RF baseline: %v", err)
		}
	})
	if err := NewHost().UpdateScenarioStep(ctx, deployment, scenario.Name, 0); err != nil {
		t.Fatal(err)
	}
	if err := NewHost().UpdateScenarioStep(ctx, deployment, scenario.Name, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := hwsim.NewClient(transport).MediumHealth(ctx); err != nil {
		t.Fatalf("medium health after dynamic update: %v", err)
	}
}

func TestUpdateScenarioStepUsesPlannedEndpointsAndConfirmsResult(t *testing.T) {
	var arguments []string
	host := Host{Transport: hostCommandFunc(func(_ context.Context, input []byte, args ...string) (int, error) {
		if !strings.Contains(string(input), "wmediumd.sock") {
			t.Fatalf("link update script missing private host socket")
		}
		arguments = append([]string(nil), args...)
		return 0, nil
	})}
	scenario := plan.WirelessScenario{
		Name: "crossover", Station: plan.ScenarioStation{MAC: "02:00:00:00:ee:01"},
		Steps: []plan.ScenarioStep{{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:02"}, SNRDb: -75}},
	}
	deployment := plan.Deployment{Name: "edge", WirelessScenarios: []plan.WirelessScenario{scenario}}
	if err := host.UpdateScenarioStep(context.Background(), deployment, "crossover", 0); err != nil {
		t.Fatal(err)
	}
	if len(arguments) != 5 || arguments[0] != "python3" || arguments[1] != "-" || !reflect.DeepEqual(arguments[2:], []string{"02:00:00:00:ee:01", "02:00:00:00:ee:02", "-75"}) {
		t.Fatalf("privileged link update arguments = %v", arguments)
	}
	arguments = nil
	for _, request := range []struct {
		name  string
		index int
	}{
		{"other-deployment", 0}, {"crossover", 1},
	} {
		if err := host.UpdateScenarioStep(context.Background(), deployment, request.name, request.index); err == nil {
			t.Fatalf("unplanned scenario step %q[%d] was accepted", request.name, request.index)
		}
	}
	if len(arguments) != 0 {
		t.Fatalf("unplanned link reached host transport: %v", arguments)
	}
	host.Transport = hostCommandFunc(func(context.Context, []byte, ...string) (int, error) { return 1, nil })
	if err := host.UpdateScenarioStep(context.Background(), deployment, "crossover", 0); err == nil {
		t.Fatal("daemon rejected update but adapter reported success")
	}
}

func TestRestoreScenarioLinksUpdatesOnlyDistinctPlannedLinks(t *testing.T) {
	var commands [][]string
	host := Host{Transport: hostCommandFunc(func(_ context.Context, _ []byte, args ...string) (int, error) {
		commands = append(commands, append([]string(nil), args...))
		return 0, nil
	})}
	scenario := plan.WirelessScenario{Name: "crossover", Station: plan.ScenarioStation{MAC: "02:00:00:00:ee:01"}, Steps: []plan.ScenarioStep{
		{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:02"}, SNRDb: -100},
		{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:03"}, SNRDb: 35},
		{AP: plan.RoamingCandidate{BSSID: "02:00:00:00:ee:02"}, SNRDb: 35},
	}}
	deployment := plan.Deployment{Name: "edge", WirelessScenarios: []plan.WirelessScenario{scenario}}
	if err := host.RestoreScenarioLinks(context.Background(), deployment, scenario.Name); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"python3", "-", scenario.Station.MAC, "02:00:00:00:ee:02", "35"}, {"python3", "-", scenario.Station.MAC, "02:00:00:00:ee:03", "35"}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("RF restore operations = %v, want %v", commands, want)
	}
	commands = nil
	if err := host.RestoreScenarioLinks(context.Background(), deployment, "unknown"); err == nil || len(commands) != 0 {
		t.Fatalf("unplanned restore reached host: operations=%v, error=%v", commands, err)
	}
}

func TestClearBaselineStopsBeforeRemovingPrivateConfiguration(t *testing.T) {
	var operations []string
	host := Host{Transport: hostCommandFunc(func(_ context.Context, _ []byte, args ...string) (int, error) {
		operations = append(operations, strings.Join(args, " "))
		return 0, nil
	})}
	if err := host.ClearBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"systemctl stop wmediumd.service", "rm -f " + baselinePath}
	if !reflect.DeepEqual(operations, want) {
		t.Fatalf("clear operations = %v, want %v", operations, want)
	}
	operations = nil
	host.Transport = hostCommandFunc(func(_ context.Context, _ []byte, args ...string) (int, error) {
		operations = append(operations, args[0])
		return 1, nil
	})
	if err := host.ClearBaseline(context.Background()); err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("stop failure = %v", err)
	}
	if !reflect.DeepEqual(operations, []string{"systemctl"}) {
		t.Fatalf("configuration removed after stop failure: %v", operations)
	}
}

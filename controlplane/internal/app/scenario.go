package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/rfmedium"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
)

type scenarioRFController interface {
	InstallBaseline(context.Context, string) error
	RestoreBaseline(context.Context, string) error
	RestoreScenarioLinks(context.Context, plan.Deployment, string) error
	UpdateScenarioStep(context.Context, plan.Deployment, string, int) error
	CheckHealthy(context.Context) error
}

type scenarioInspector interface {
	wirelessReadinessInspector
	ObserveStation(context.Context, string, string) (podman.StationObservation, error)
}

type scenarioOutcome struct {
	initialBSSID string
	finalBSSID   string
	ipv4         netip.Prefix
	roam         time.Duration
	maxOutage    time.Duration
	maxInterval  time.Duration
	samples      int
	lastBSSID    string
	lastAuth     bool
	timedSteps   int
}

const scenarioSamplePeriod = 200 * time.Millisecond

func runScenario(opts Options) (daemon.CommandResponse, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lock, err := state.AcquireWriterLock(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer lock.Release()
	store, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer store.Close()
	plans, err := rfBaselinePlansAfterDown(store, "")
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	var deployment *plan.Deployment
	var scenario *plan.WirelessScenario
	for index := range plans {
		if plans[index].Name != opts.Name {
			continue
		}
		deployment = &plans[index]
		for scenarioIndex := range deployment.WirelessScenarios {
			if deployment.WirelessScenarios[scenarioIndex].Name == opts.Scenario {
				scenario = &deployment.WirelessScenarios[scenarioIndex]
			}
		}
	}
	if deployment == nil || scenario == nil {
		return daemon.CommandResponse{}, fmt.Errorf("scenario %q is not declared in active deployment %q", opts.Scenario, opts.Name)
	}
	baseline, err := rfmedium.BuildBaseline(plans)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	outcome, err := executeWirelessScenario(ctx, *deployment, *scenario, baseline, rfmedium.NewHost(), podman.New())
	if err != nil {
		return daemon.CommandResponse{}, fmt.Errorf("scenario %s/%s: %w", opts.Name, opts.Scenario, err)
	}
	return daemon.CommandResponse{Message: fmt.Sprintf("scenario %s/%s passed (controlled lab): %s -> %s in %s; IPv4 %s unchanged; maximum bounded probe outage %s (longest sample interval %s, %d samples)", opts.Name, opts.Scenario, outcome.initialBSSID, outcome.finalBSSID, outcome.roam, outcome.ipv4, outcome.maxOutage, outcome.maxInterval, outcome.samples)}, nil
}

func scenarioTarget(deployment plan.Deployment, scenario plan.WirelessScenario) (string, string, string, error) {
	var container, device, root string
	for _, service := range deployment.Services {
		for _, bridge := range service.Bridges {
			if bridge.DHCPStart != "" && bridge.DHCPEnd != "" {
				prefix, err := netip.ParsePrefix(bridge.IPv4)
				if err != nil || root != "" {
					return "", "", "", fmt.Errorf("scenario requires one valid root DHCP bridge")
				}
				root = prefix.Addr().String()
			}
		}
		if service.Name != scenario.Station.Service || !isPurposeBuiltWirelessClient(service.Image.Repository) {
			continue
		}
		for _, instance := range service.Instances {
			if instance.Index != scenario.Station.Replica {
				continue
			}
			for _, radio := range instance.Radios {
				if radio.Name == scenario.Station.Radio && radio.Mode == "station" && len(radio.RoamingMedia) > 0 {
					container = instance.PodmanContainerName(deployment.Name, service.Name)
					device = radio.Device
				}
			}
		}
	}
	if container == "" || device == "" || root == "" {
		return "", "", "", fmt.Errorf("scenario requires a roaming wireless-client and one root DHCP bridge")
	}
	return container, device, root, nil
}

func executeWirelessScenario(ctx context.Context, deployment plan.Deployment, scenario plan.WirelessScenario, baseline string, host scenarioRFController, inspector scenarioInspector) (outcome scenarioOutcome, result error) {
	container, device, root, err := scenarioTarget(deployment, scenario)
	if err != nil {
		return outcome, err
	}
	if len(scenario.Steps) == 0 || scenario.Assertions.MaxRoamMs <= 0 || scenario.Assertions.MaxGapMs <= 0 {
		return outcome, fmt.Errorf("scenario has no valid steps or bounds")
	}
	if err := waitForGatewayVAPs(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	if err := waitForGatewayMeshes(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	if err := waitForRemoteMeshDHCP(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	if err := waitForWirelessAuthentication(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := host.RestoreScenarioLinks(cleanupCtx, deployment, scenario.Name); err != nil {
			result = errors.Join(result, fmt.Errorf("restore scenario RF links: %w", err))
			if fallbackErr := host.RestoreBaseline(cleanupCtx, baseline); fallbackErr != nil {
				result = errors.Join(result, fmt.Errorf("recover RF baseline daemon: %w", fallbackErr))
			}
		}
	}()
	if err := host.InstallBaseline(ctx, baseline); err != nil {
		return outcome, fmt.Errorf("prepare RF baseline: %w", err)
	}
	if err := host.RestoreScenarioLinks(ctx, deployment, scenario.Name); err != nil {
		return outcome, fmt.Errorf("prepare scenario RF links: %w", err)
	}
	if err := host.CheckHealthy(ctx); err != nil {
		return outcome, err
	}
	for index, step := range scenario.Steps {
		if step.AtMs != 0 {
			break
		}
		if err := host.UpdateScenarioStep(ctx, deployment, scenario.Name, index); err != nil {
			return outcome, err
		}
	}
	if err := waitForGatewayMeshes(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	if err := waitForRemoteMeshDHCP(ctx, deployment, inspector); err != nil {
		return outcome, err
	}
	initial, err := waitForScenarioStart(ctx, host, inspector, container, device, root, scenario.Assertions.InitialAP.BSSID)
	if err != nil {
		return outcome, err
	}
	if !scenario.Assertions.SameIPv4 {
		return outcome, fmt.Errorf("scenario requires IPv4 continuity assertion")
	}
	outcome, err = runTimedScenario(ctx, deployment, scenario, host, inspector, container, device, root, initial.IPv4)
	if err != nil {
		return outcome, fmt.Errorf("%w (last sample: authenticated=%t BSSID=%q IPv4=%s; %d samples; %d timed steps)", err, outcome.lastAuth, outcome.lastBSSID, outcome.ipv4, outcome.samples, outcome.timedSteps)
	}
	return outcome, nil
}

func waitForScenarioStart(ctx context.Context, host scenarioRFController, inspector scenarioInspector, container, device, root, initialBSSID string) (podman.StationObservation, error) {
	waitCtx, cancel := context.WithTimeout(ctx, wirelessAuthenticationTimeout)
	defer cancel()
	ticker := time.NewTicker(wirelessAuthenticationPoll)
	defer ticker.Stop()
	for {
		if err := host.CheckHealthy(waitCtx); err != nil {
			return podman.StationObservation{}, err
		}
		observation, err := inspector.ObserveStation(waitCtx, container, device)
		if err == nil && observation.Authenticated && strings.EqualFold(observation.BSSID, initialBSSID) && observation.IPv4.IsValid() {
			if err := inspector.ProbeLAN(waitCtx, container, device, root); err == nil {
				return observation, nil
			}
		}
		select {
		case <-waitCtx.Done():
			return podman.StationObservation{}, fmt.Errorf("initial AP %s and LAN probe not ready: %w", initialBSSID, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

type scenarioSample struct {
	at          time.Time
	station     podman.StationObservation
	probePassed bool
	err         error
}

func runTimedScenario(ctx context.Context, deployment plan.Deployment, scenario plan.WirelessScenario, host scenarioRFController, inspector scenarioInspector, container, device, root string, initialIPv4 netip.Prefix) (scenarioOutcome, error) {
	sampleCtx, cancel := context.WithCancel(ctx)
	samples := make(chan scenarioSample, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(scenarioSamplePeriod)
		defer ticker.Stop()
		for {
			observation, err := inspector.ObserveStation(sampleCtx, container, device)
			if err == nil {
				err = host.CheckHealthy(sampleCtx)
			}
			probePassed := err == nil && inspector.ProbeLAN(sampleCtx, container, device, root) == nil
			select {
			case samples <- scenarioSample{at: time.Now(), station: observation, probePassed: probePassed, err: err}:
			case <-sampleCtx.Done():
				return
			}
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() { cancel(); <-done }()
	start := time.Now()
	lastGood := start
	var crossover, roamedAt time.Time
	var finalProbe bool
	var outage bool
	outcome := scenarioOutcome{initialBSSID: scenario.Assertions.InitialAP.BSSID, finalBSSID: scenario.Assertions.FinalAP.BSSID, ipv4: initialIPv4}
	consume := func(sample scenarioSample) error {
		outcome.samples++
		outcome.lastBSSID = sample.station.BSSID
		outcome.lastAuth = sample.station.Authenticated
		if sample.err != nil {
			return sample.err
		}
		if sample.station.IPv4 != initialIPv4 {
			return fmt.Errorf("station IPv4 changed from %s to %s", initialIPv4, sample.station.IPv4)
		}
		if sample.probePassed {
			gap := sample.at.Sub(lastGood)
			if gap > outcome.maxInterval {
				outcome.maxInterval = gap
			}
			if outage && gap > outcome.maxOutage {
				outcome.maxOutage = gap
			}
			outage = false
			if gap > time.Duration(scenario.Assertions.MaxGapMs)*time.Millisecond {
				return fmt.Errorf("LAN probe gap exceeded %d ms", scenario.Assertions.MaxGapMs)
			}
			lastGood = sample.at
		} else {
			outage = true
			if sample.at.Sub(lastGood) > time.Duration(scenario.Assertions.MaxGapMs)*time.Millisecond {
				return fmt.Errorf("LAN probe outage exceeded %d ms", scenario.Assertions.MaxGapMs)
			}
		}
		if (crossover.IsZero() || sample.at.Before(crossover)) && sample.station.Authenticated && !strings.EqualFold(sample.station.BSSID, scenario.Assertions.InitialAP.BSSID) {
			return fmt.Errorf("station left initial AP before timed crossover")
		}
		if !crossover.IsZero() && !sample.at.Before(crossover) && sample.station.Authenticated && strings.EqualFold(sample.station.BSSID, scenario.Assertions.FinalAP.BSSID) {
			if roamedAt.IsZero() {
				roamedAt = sample.at
				outcome.roam = roamedAt.Sub(crossover)
			}
			finalProbe = sample.probePassed
		} else {
			finalProbe = false
		}
		if !roamedAt.IsZero() && roamedAt.Sub(crossover) > time.Duration(scenario.Assertions.MaxRoamMs)*time.Millisecond {
			return fmt.Errorf("automatic roam exceeded %d ms", scenario.Assertions.MaxRoamMs)
		}
		return nil
	}
	waitUntil := func(deadline time.Time) error {
		for {
			if !crossover.IsZero() && roamedAt.IsZero() {
				roamDeadline := crossover.Add(time.Duration(scenario.Assertions.MaxRoamMs) * time.Millisecond)
				if !time.Now().Before(roamDeadline) {
					return fmt.Errorf("automatic roam to %s timed out after %d ms", scenario.Assertions.FinalAP.BSSID, scenario.Assertions.MaxRoamMs)
				}
				if roamDeadline.Before(deadline) {
					deadline = roamDeadline
				}
			}
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ctx.Err()
			}
			timer := time.NewTimer(remaining)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case sample := <-samples:
				timer.Stop()
				if err := consume(sample); err != nil {
					return err
				}
			case <-timer.C:
				if !crossover.IsZero() && roamedAt.IsZero() && !time.Now().Before(crossover.Add(time.Duration(scenario.Assertions.MaxRoamMs)*time.Millisecond)) {
					return fmt.Errorf("automatic roam to %s timed out after %d ms", scenario.Assertions.FinalAP.BSSID, scenario.Assertions.MaxRoamMs)
				}
				return nil
			}
		}
	}
	for index, step := range scenario.Steps {
		if step.AtMs == 0 {
			continue
		}
		if err := waitUntil(start.Add(time.Duration(step.AtMs) * time.Millisecond)); err != nil {
			return outcome, err
		}
		if err := host.UpdateScenarioStep(ctx, deployment, scenario.Name, index); err != nil {
			return outcome, err
		}
		outcome.timedSteps++
		if crossover.IsZero() {
			crossover = time.Now()
		}
	}
	if crossover.IsZero() {
		return outcome, fmt.Errorf("scenario has no timed signal crossover")
	}
	deadline := crossover.Add(time.Duration(scenario.Assertions.MaxRoamMs) * time.Millisecond)
	for {
		if !roamedAt.IsZero() && finalProbe {
			if err := ctx.Err(); err != nil {
				return outcome, err
			}
			return outcome, nil
		}
		if !time.Now().Before(deadline) {
			return outcome, fmt.Errorf("automatic roam to %s timed out after %d ms", scenario.Assertions.FinalAP.BSSID, scenario.Assertions.MaxRoamMs)
		}
		if err := waitUntil(time.Now().Add(min(scenarioSamplePeriod, time.Until(deadline)))); err != nil {
			return outcome, err
		}
	}
}

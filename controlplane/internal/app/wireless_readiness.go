package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

const wirelessClientStatusPath = "/run/vcpe/wireless-client/status"

var wirelessAuthenticationTimeout = 45 * time.Second
var wirelessAuthenticationPoll = 250 * time.Millisecond
var wirelessVAPTimeout = 45 * time.Second
var wirelessVAPPoll = 250 * time.Millisecond
var wirelessMeshTimeout = 45 * time.Second
var wirelessMeshPoll = 250 * time.Millisecond

type wirelessReadinessInspector interface {
	ContainerFile(context.Context, string, string) ([]byte, error)
	ContainerVAPStatus(context.Context, string) ([]byte, error)
	ContainerMeshStatus(context.Context, string) ([]byte, error)
	ProbeLAN(context.Context, string, string, string) error
}

type observedMesh struct {
	Radio     string `json:"radio"`
	Interface string `json:"interface"`
	Bridge    string `json:"bridge"`
	Joined    bool   `json:"joined"`
	Peers     int    `json:"peers"`
}

func waitForGatewayMeshes(ctx context.Context, deployment plan.Deployment, inspector wirelessReadinessInspector) error {
	expected := map[string]observedMesh{}
	for _, service := range deployment.Services {
		for _, instance := range service.Instances {
			for _, radio := range instance.Radios {
				if radio.Mesh != nil {
					container := instance.PodmanContainerName(deployment.Name, service.Name)
					expected[container] = observedMesh{Radio: radio.Name, Interface: radio.Device, Bridge: radio.Bridge, Joined: true}
				}
			}
		}
	}
	if len(expected) == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, wirelessMeshTimeout)
	defer cancel()
	ticker := time.NewTicker(wirelessMeshPoll)
	defer ticker.Stop()
	for {
		pending := map[string]bool{}
		for container, planned := range expected {
			status, err := inspector.ContainerMeshStatus(waitCtx, container)
			var observed observedMesh
			if err != nil || json.Unmarshal(status, &observed) != nil || observed.Radio != planned.Radio || observed.Interface != planned.Interface || observed.Bridge != planned.Bridge || !observed.Joined || observed.Peers < 1 {
				pending[container+" "+planned.Radio] = true
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("mesh peer readiness timed out for %s", strings.Join(sortedTrueKeys(pending), ", "))
		case <-ticker.C:
		}
	}
}

func waitForRemoteMeshDHCP(ctx context.Context, deployment plan.Deployment, inspector wirelessReadinessInspector) error {
	if len(deployment.WirelessScenarios) == 0 || !hasMeshRadios(deployment) {
		return nil
	}
	var rootPrefix netip.Prefix
	var rootAddress netip.Addr
	var leaseStart, leaseEnd netip.Addr
	for _, service := range deployment.Services {
		for _, bridge := range service.Bridges {
			if bridge.DHCPStart == "" || bridge.DHCPEnd == "" {
				continue
			}
			prefix, err := netip.ParsePrefix(bridge.IPv4)
			if err != nil {
				return fmt.Errorf("mesh root bridge %s: %w", bridge.Name, err)
			}
			rootAddress = prefix.Addr()
			rootPrefix = prefix
			leaseStart, err = netip.ParseAddr(bridge.DHCPStart)
			if err != nil {
				return fmt.Errorf("mesh root DHCP start: %w", err)
			}
			leaseEnd, err = netip.ParseAddr(bridge.DHCPEnd)
			if err != nil {
				return fmt.Errorf("mesh root DHCP end: %w", err)
			}
		}
	}
	if !rootAddress.IsValid() {
		return fmt.Errorf("mesh root DHCP bridge is missing")
	}
	for _, scenario := range deployment.WirelessScenarios {
		var container, device, radioName string
		for _, service := range deployment.Services {
			if !isPurposeBuiltWirelessClient(service.Image.Repository) {
				continue
			}
			for _, instance := range service.Instances {
				if len(instance.Interfaces) != 0 {
					continue
				}
				for _, radio := range instance.Radios {
					if radio.Mode == "station" && radio.APBSSID == scenario.Assertions.FinalAP.BSSID && len(radio.RoamingMedia) == 0 {
						if container != "" {
							return fmt.Errorf("mesh scenario %q has multiple remote DHCP probes", scenario.Name)
						}
						container = instance.PodmanContainerName(deployment.Name, service.Name)
						device, radioName = radio.Device, radio.Name
					}
				}
			}
		}
		if container == "" {
			return fmt.Errorf("mesh scenario %q requires a radio-only wireless client fixed to the final AP for remote DHCP readiness", scenario.Name)
		}
		waitCtx, cancel := context.WithTimeout(ctx, wirelessMeshTimeout)
		ticker := time.NewTicker(wirelessMeshPoll)
		ready := false
		lastFailure := "station status unavailable"
		for !ready {
			status, err := inspector.ContainerFile(waitCtx, container, wirelessClientStatusPath)
			if err == nil {
				fields := map[string]string{}
				for _, line := range strings.Split(string(status), "\n") {
					key, value, ok := strings.Cut(line, "=")
					if ok {
						fields[key] = value
					}
				}
				lease, parseErr := netip.ParsePrefix(fields["ipv4"])
				switch {
				case fields["state"] != "ready" || !strings.EqualFold(fields["radio"], strings.ReplaceAll(radioName, "-", "_")) || fields["device"] != device || !strings.EqualFold(fields["bssid"], scenario.Assertions.FinalAP.BSSID):
					lastFailure = "station is not authenticated on expected AP"
				case parseErr != nil || lease.Bits() != rootPrefix.Bits() || lease.Masked() != rootPrefix.Masked() || lease.Addr().Compare(leaseStart) < 0 || lease.Addr().Compare(leaseEnd) > 0:
					lastFailure = "lease outside root DHCP range"
				default:
					lastFailure = "LAN probe failed"
					ready = inspector.ProbeLAN(waitCtx, container, device, rootAddress.String()) == nil
				}
			}
			if ready {
				break
			}
			select {
			case <-waitCtx.Done():
				ticker.Stop()
				cancel()
				return fmt.Errorf("remote-root DHCP readiness timed out for %s radio %s on AP %s (%s): %w", container, radioName, scenario.Assertions.FinalAP.BSSID, lastFailure, waitCtx.Err())
			case <-ticker.C:
			}
		}
		ticker.Stop()
		cancel()
	}
	return nil
}

type observedVAP struct {
	Radio     string `json:"radio"`
	Slot      int    `json:"slot"`
	Interface string `json:"interface"`
	BSSID     string `json:"bssid"`
	Enabled   bool   `json:"enabled"`
	Bridge    string `json:"bridge"`
}

func waitForGatewayVAPs(ctx context.Context, deployment plan.Deployment, inspector wirelessReadinessInspector) error {
	expected := map[string]map[string]observedVAP{}
	for _, service := range deployment.Services {
		for _, instance := range service.Instances {
			container := instance.PodmanContainerName(deployment.Name, service.Name)
			for _, radio := range instance.Radios {
				if radio.Mode != "ap" || radio.Medium == "" {
					continue
				}
				for _, vap := range radio.VAPs {
					if expected[container] == nil {
						expected[container] = map[string]observedVAP{}
					}
					key := fmt.Sprintf("%s slot %d", radio.Name, vap.Slot)
					expected[container][key] = observedVAP{Radio: radio.Name, Slot: vap.Slot, Interface: vap.Device, BSSID: vap.MAC, Enabled: true, Bridge: vap.Bridge}
				}
			}
		}
	}
	if len(expected) == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, wirelessVAPTimeout)
	defer cancel()
	ticker := time.NewTicker(wirelessVAPPoll)
	defer ticker.Stop()
	for {
		pending := map[string]bool{}
		for container, targets := range expected {
			status, err := inspector.ContainerVAPStatus(waitCtx, container)
			observed := map[string]observedVAP{}
			if len(status) > 0 {
				decoder := json.NewDecoder(bytes.NewReader(status))
				for {
					var vap observedVAP
					if decodeErr := decoder.Decode(&vap); decodeErr != nil {
						if decodeErr != io.EOF {
							observed = map[string]observedVAP{}
						}
						break
					}
					key := fmt.Sprintf("%s slot %d", vap.Radio, vap.Slot)
					if _, duplicate := observed[key]; duplicate {
						observed = map[string]observedVAP{}
						break
					}
					observed[key] = vap
				}
			}
			for key, planned := range targets {
				actual, exists := observed[key]
				if !exists || actual != planned {
					pending[container+" "+key] = true
				}
			}
			if err != nil && len(observed) == len(targets) {
				for key := range targets {
					pending[container+" "+key] = true
				}
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wireless VAP readiness timed out for %s", strings.Join(sortedTrueKeys(pending), ", "))
		case <-ticker.C:
		}
	}
}

func waitForWirelessAuthentication(ctx context.Context, deployment plan.Deployment, inspector wirelessReadinessInspector) error {
	containers := personalWirelessClientContainers(deployment)
	if len(containers) == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, wirelessAuthenticationTimeout)
	defer cancel()
	pending := make(map[string]bool, len(containers))
	for _, container := range containers {
		pending[container] = true
	}
	ticker := time.NewTicker(wirelessAuthenticationPoll)
	defer ticker.Stop()
	for {
		for container := range pending {
			status, err := inspector.ContainerFile(waitCtx, container, wirelessClientStatusPath)
			if err == nil && strings.Contains(string(status), "state=ready\n") {
				delete(pending, container)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wireless authentication readiness timed out for %s", strings.Join(sortedTrueKeys(pending), ", "))
		case <-ticker.C:
		}
	}
}

func personalWirelessClientContainers(deployment plan.Deployment) []string {
	containers := map[string]bool{}
	for _, service := range deployment.Services {
		if !isPurposeBuiltWirelessClient(service.Image.Repository) {
			continue
		}
		for _, instance := range service.Instances {
			for _, radio := range instance.Radios {
				wireless := deployment.WirelessNetwork(radio.Network)
				if radio.Mode != "station" || wireless == nil || !isPersonalWireless(wireless.Security) {
					continue
				}
				containers[instance.PodmanContainerName(deployment.Name, service.Name)] = true
			}
		}
	}
	return sortedTrueKeys(containers)
}

func isPurposeBuiltWirelessClient(repository string) bool {
	return repository == "wireless-client" || strings.HasSuffix(repository, "/wireless-client")
}

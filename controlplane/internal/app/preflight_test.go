package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/hwsim"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
)

type preflightTestRenderer struct{}

func (preflightTestRenderer) Name() string { return "preflight-test-renderer" }

func (preflightTestRenderer) Render(context.Context, render.Input) (render.Result, error) {
	return render.Result{}, nil
}

type unrestrictedReplicaType struct {
	typeregistry.BaseServiceType
	name string
}

func (serviceType unrestrictedReplicaType) Type() string                      { return serviceType.name }
func (unrestrictedReplicaType) ValidateConfig(yaml.Node) error                { return nil }
func (unrestrictedReplicaType) Renderer() render.Renderer                     { return preflightTestRenderer{} }
func (unrestrictedReplicaType) ExpectedRoles() []typeregistry.RoleRequirement { return nil }
func (unrestrictedReplicaType) Description() string                           { return "preflight test type" }
func (unrestrictedReplicaType) DefaultImage() string                          { return "example.invalid/test" }

type singletonReplicaType struct {
	unrestrictedReplicaType
}

func (singletonReplicaType) ValidateReplicas(replicas int) error {
	if replicas > 1 {
		return fmt.Errorf("embedded topology is singleton; got %d replicas", replicas)
	}
	return nil
}

func TestPreflightOptionalReplicaValidation(t *testing.T) {
	typeregistry.Register(unrestrictedReplicaType{name: "preflight-unrestricted"})
	typeregistry.Register(singletonReplicaType{unrestrictedReplicaType{name: "preflight-singleton"}})

	document := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{Services: []manifest.Service{{
			Name:     "subscriber",
			Type:     "preflight-unrestricted",
			Replicas: 2,
		}}},
	}
	if err := Preflight(document); err != nil {
		t.Fatalf("Preflight() unrestricted type error = %v", err)
	}

	document.Spec.Services[0].Type = "preflight-singleton"
	err := Preflight(document)
	if err == nil || !strings.Contains(err.Error(), `service "subscriber" replicas: embedded topology is singleton; got 2 replicas`) {
		t.Fatalf("Preflight() singleton error = %v", err)
	}
}

func TestPreflightRejectsConflictingMeshLANConfig(t *testing.T) {
	types.Register()
	manifestYAML := `apiVersion: vcpe.dev/v1
kind: Deployment
metadata: {name: edge}
spec:
	networks: []
	wirelessMedia:
		- {name: mesh-rf, band: 5ghz, channel: 36, widthMHz: 20}
		- {name: root-rf, band: 5ghz, channel: 40, widthMHz: 20}
		- {name: remote-rf, band: 5ghz, channel: 40, widthMHz: 20}
	wirelessNetworks: [{name: home, ssid: home, security: open}]
	secrets: [{name: mesh-key, provider: env, key: VCPE_MESH_KEY}]
	services:
		- name: root
			type: gateway
			replicas: 1
			image: {repository: gateway}
			bridges: [{name: brlan0, ipv4: 10.0.0.1/24, dhcpStart: 10.0.0.100, dhcpEnd: 10.0.0.200}]
			radios:
				- {name: mesh, mode: mesh, medium: mesh-rf, device: mesh0, mesh: {id: lab-mesh, bridge: brlan0, saeSecretRef: mesh-key}}
				- {name: ap, mode: ap, medium: root-rf, device: wlan0, vaps: [{slot: 0, network: home, bridge: brlan0}]}
			config:
				lan: {dhcpStart: 10.0.0.50}
		- name: remote
			type: gateway
			replicas: 1
			image: {repository: gateway}
			bridges: [{name: brlan0}]
			radios:
				- {name: mesh, mode: mesh, medium: mesh-rf, device: mesh0, mesh: {id: lab-mesh, bridge: brlan0, saeSecretRef: mesh-key}}
				- {name: ap, mode: ap, medium: remote-rf, device: wlan0, vaps: [{slot: 0, network: home, bridge: brlan0}]}
`
	manifestYAML = strings.ReplaceAll(manifestYAML, "\t", "  ")
	doc, err := manifest.Parse([]byte(manifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := Preflight(doc); err == nil || !strings.Contains(err.Error(), "config.lan.dhcpStart conflicts") {
		t.Fatalf("Preflight() error = %v, want conflicting mesh DHCP configuration", err)
	}
	doc, err = manifest.Parse([]byte(strings.Replace(manifestYAML, "dhcpStart: 10.0.0.50", "dhcpStart: 10.0.0.100", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Preflight(doc); err != nil {
		t.Fatalf("matching legacy mesh configuration should validate: %v", err)
	}
}

func TestPreflightWebConfigRequirements(t *testing.T) {
	types.Register()
	document := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{{Role: "mgmt"}},
			Services: []manifest.Service{{
				Name: "webconfig", Type: "webconfig", Replicas: 1,
			}},
		},
	}
	if err := Preflight(document); err == nil || !strings.Contains(err.Error(), `does not satisfy expected role "mgmt"`) {
		t.Fatalf("Preflight() missing management interface error = %v", err)
	}

	document.Spec.Services[0].Interfaces = []manifest.Interface{{Role: "mgmt"}}
	document.Spec.Services[0].Replicas = 2
	if err := Preflight(document); err == nil || !strings.Contains(err.Error(), "WebConfig is singleton; got 2 replicas") {
		t.Fatalf("Preflight() multiple WebConfig replicas error = %v", err)
	}
}

func TestMeshGatewayRequiresPinnedImageWithoutChangingLegacyImages(t *testing.T) {
	const digest = "ghcr.io/gdcs-dev/gateway@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name, mode, repository, tag, environment, context, policy string
		wantError                                                 bool
	}{
		{"mesh pinned", manifest.RadioModeMesh, digest, "", "", "", "", false},
		{"mesh mutable tag", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "", "", "", true},
		{"development local build", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "development", "services/gateway", "build-if-missing", false},
		{"production local build", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "production", "services/gateway", "build-if-missing", true},
		{"development remote tag", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "development", "", "build-if-missing", true},
		{"development wrong context", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "development", "services/other", "build-if-missing", true},
		{"development wrong policy", manifest.RadioModeMesh, "ghcr.io/gdcs-dev/gateway", "dev", "development", "services/gateway", "always-pull", true},
		{"mesh wrong publisher", manifest.RadioModeMesh, strings.Replace(digest, "gdcs-dev", "untrusted", 1), "", "", "", "", true},
		{"mesh digest and tag", manifest.RadioModeMesh, digest, "dev", "", "", "", true},
		{"legacy AP tag", manifest.RadioModeAP, "ghcr.io/gdcs-dev/gateway", "dev", "", "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := manifest.Document{Metadata: manifest.Metadata{Labels: map[string]string{"environment": test.environment}}, Spec: manifest.Spec{Services: []manifest.Service{{
				Name: "gateway", Type: "gateway", Image: manifest.Image{Repository: test.repository, Tag: test.tag, BuildContext: test.context, PullPolicy: test.policy},
				Radios: []manifest.Radio{{Mode: test.mode}},
			}}}}
			err := requirePinnedMeshGatewayImages(document)
			if test.wantError && err == nil || !test.wantError && err != nil {
				t.Fatalf("image preflight error = %v, want rejection = %t", err, test.wantError)
			}
		})
	}
}

type meshImageVerifierFunc func(context.Context, string) error

func (verify meshImageVerifierFunc) VerifyMeshGatewayImage(ctx context.Context, reference string) error {
	return verify(ctx, reference)
}

type meshCapableHWSIMTransport struct{ healthyHWSIMTransport }

func (transport meshCapableHWSIMTransport) Execute(ctx context.Context, args ...string) (hwsim.CommandResult, error) {
	if args[0] == "doctor" {
		return hwsim.CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"meshCapability":"supported","checks":[]}}`)}, nil
	}
	return transport.healthyHWSIMTransport.Execute(ctx, args...)
}

func TestVerifyMeshGatewayImagesBeforeWorkloadMutation(t *testing.T) {
	const reference = "ghcr.io/gdcs-dev/gateway@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	document := manifest.Document{Spec: manifest.Spec{Services: []manifest.Service{{
		Name: "root", Type: "gateway", Image: manifest.Image{Repository: reference},
		Radios: []manifest.Radio{{Mode: manifest.RadioModeMesh}},
	}, {
		Name: "other", Type: "gateway", Image: manifest.Image{Repository: "gateway", Tag: "dev"},
		Radios: []manifest.Radio{{Mode: manifest.RadioModeAP}},
	}}}}
	var calls []string
	verifier := meshImageVerifierFunc(func(_ context.Context, ref string) error {
		calls = append(calls, ref)
		return fmt.Errorf("unsupported Gateway image")
	})
	if err := verifyMeshGatewayImages(context.Background(), document, verifier); err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("mesh image verification = %v", err)
	}
	if len(calls) != 1 || calls[0] != reference {
		t.Fatalf("verified image references = %v", calls)
	}
	document.Spec.Services = document.Spec.Services[1:]
	if err := verifyMeshGatewayImages(context.Background(), document, verifier); err != nil || len(calls) != 1 {
		t.Fatalf("legacy image verification error = %v, calls = %v", err, calls)
	}
}

func TestApplyRejectsUnsupportedMeshCapabilitiesBeforeAllocation(t *testing.T) {
	types.Register()
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_MESH_KEY", "12345678")
	previousClient := newHWSIMClient
	previousVerifier := newMeshGatewayImageVerifier
	previousNetwork := newNetworkProvisioner
	previousCompose := newComposeRunner
	previousManager := newWirelessManager
	var networkCalls, composeCalls, radioCalls int
	newHWSIMClient = func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(meshCapableHWSIMTransport{}), nil
	}
	newMeshGatewayImageVerifier = func() meshGatewayImageVerifier {
		return meshImageVerifierFunc(func(context.Context, string) error { return fmt.Errorf("unsupported Gateway image") })
	}
	newNetworkProvisioner = func() networkProvisioner {
		networkCalls++
		return &recordingNetworkProvisioner{}
	}
	newComposeRunner = func() composeLifecycleRunner {
		composeCalls++
		return &credentialComposeRunner{}
	}
	newWirelessManager = func(context.Context) (wirelessRadioManager, error) {
		radioCalls++
		return nil, fmt.Errorf("radio attachment must not start")
	}
	t.Cleanup(func() {
		newHWSIMClient = previousClient
		newMeshGatewayImageVerifier = previousVerifier
		newNetworkProvisioner = previousNetwork
		newComposeRunner = previousCompose
		newWirelessManager = previousManager
	})

	const digest = "ghcr.io/gdcs-dev/gateway@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	doc := manifest.Document{APIVersion: manifest.APIVersion, Kind: manifest.Kind, Metadata: manifest.Metadata{Name: "edge"}, Spec: manifest.Spec{
		WirelessMedia:    []manifest.WirelessMedium{{Name: "backhaul", Band: "5ghz", Channel: 36, WidthMHz: 20}, {Name: "access-root", Band: "5ghz", Channel: 40, WidthMHz: 20}, {Name: "access-remote", Band: "5ghz", Channel: 40, WidthMHz: 20}},
		WirelessNetworks: []manifest.WirelessNetwork{{Name: "home", SSID: "home", Security: "open"}},
		Secrets:          []manifest.SecretRef{{Name: "mesh-key", Provider: "env", Key: "VCPE_MESH_KEY"}},
	}}
	for _, name := range []string{"root", "remote"} {
		bridge := manifest.BridgeSpec{Name: "brlan0"}
		accessMedium := "access-" + name
		if name == "root" {
			bridge.IPv4, bridge.DHCPStart, bridge.DHCPEnd = "10.0.0.1/24", "10.0.0.100", "10.0.0.200"
		}
		doc.Spec.Services = append(doc.Spec.Services, manifest.Service{
			Name: name, Type: "gateway", Replicas: 1, Image: manifest.Image{Repository: digest}, Bridges: []manifest.BridgeSpec{bridge},
			Radios: []manifest.Radio{
				{Name: "mesh", Mode: manifest.RadioModeMesh, Medium: "backhaul", Device: "mesh0", Mesh: &manifest.Mesh{ID: "lab-mesh", Bridge: "brlan0", SAESecretRef: "mesh-key"}},
				{Name: "ap", Mode: manifest.RadioModeAP, Medium: accessMedium, Device: "wlan0", VAPs: []manifest.VAP{{Slot: 0, Network: "home", Bridge: "brlan0"}}},
			},
		})
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mesh.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	_, err = runApply(Options{ManifestPath: path, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unsupported Gateway image") {
		t.Fatalf("apply image error = %v", err)
	}
	if networkCalls != 0 || composeCalls != 0 || radioCalls != 0 {
		t.Fatalf("mutation reached networks = %d, compose = %d, radios = %d", networkCalls, composeCalls, radioCalls)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if deployments, err := store.ListKnownDeployments(); err != nil || len(deployments) != 0 {
		t.Fatalf("persisted deployments = %v, error = %v", deployments, err)
	}

	newMeshGatewayImageVerifier = func() meshGatewayImageVerifier {
		return meshImageVerifierFunc(func(context.Context, string) error { return nil })
	}
	newHWSIMClient = func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(healthyHWSIMTransport{}), nil
	}
	_, err = runApply(Options{ManifestPath: path, StateRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "mesh capability") {
		t.Fatalf("old API-v1 manager error = %v", err)
	}
	if networkCalls != 0 || composeCalls != 0 || radioCalls != 0 {
		t.Fatalf("old manager reached networks = %d, compose = %d, radios = %d", networkCalls, composeCalls, radioCalls)
	}
}

type rfHealthTransport struct {
	installed *bool
	commands  *[]string
}

func (transport rfHealthTransport) Execute(ctx context.Context, args ...string) (hwsim.CommandResult, error) {
	*transport.commands = append(*transport.commands, args[0])
	if args[0] == "medium-health" {
		if !*transport.installed {
			return hwsim.CommandResult{}, fmt.Errorf("health checked before baseline")
		}
		return hwsim.CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"medium-health","success":true,"data":{"healthy":true,"checks":[{"name":"baseline","ok":true},{"name":"service","ok":true},{"name":"socket","ok":true},{"name":"registration","ok":true}]}}`)}, nil
	}
	return meshCapableHWSIMTransport{}.Execute(ctx, args...)
}

type rfBaselineInstallerFunc func(context.Context, string) error

func (install rfBaselineInstallerFunc) InstallBaseline(ctx context.Context, config string) error {
	return install(ctx, config)
}

func TestApplyWirelessPreflightInstallsBaselineBeforeHealth(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	resolved := plan.Deployment{Name: "edge", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:00:00:01"}, {MAC: "02:00:00:00:00:02"}}}}}}}
	document := manifest.Document{Spec: manifest.Spec{WirelessScenarios: []manifest.WirelessScenario{{Name: "crossover"}}}}
	installed := false
	var commands []string
	client := func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(rfHealthTransport{installed: &installed, commands: &commands}), nil
	}
	installer := rfBaselineInstallerFunc(func(_ context.Context, config string) error {
		if !strings.Contains(config, "(0,1,35)") {
			t.Fatalf("baseline missing link: %s", config)
		}
		installed = true
		return nil
	})
	if err := preflightWirelessForApply(context.Background(), store, document, resolved, client, installer); err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "version,doctor,version,doctor,medium-health" {
		t.Fatalf("preflight commands = %v", commands)
	}
	installed, commands = false, nil
	document.Spec.WirelessScenarios = nil
	if err := preflightWirelessForApply(context.Background(), store, document, resolved, client, installer); err != nil {
		t.Fatal(err)
	}
	if installed || strings.Contains(strings.Join(commands, ","), "medium-health") {
		t.Fatalf("legacy preflight installed RF baseline = %t, commands = %v", installed, commands)
	}
}

func TestLegacyApplyPreservesAnotherActiveRFScenario(t *testing.T) {
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	doc, err := manifest.Parse([]byte(wirelessSnapshot("other")))
	if err != nil {
		t.Fatal(err)
	}
	otherAP := doc.Spec.Services[0].Radios[0]
	otherAP.Name, otherAP.Device = "other-ap", "wlan1"
	doc.Spec.Services[0].Radios = append(doc.Spec.Services[0].Radios, otherAP)
	doc.Spec.Services = append(doc.Spec.Services, manifest.Service{
		Name: "station", Type: "generic-container", Replicas: 1, Image: manifest.Image{Repository: "wireless-client"},
		Radios: []manifest.Radio{{Name: "station", Mode: manifest.RadioModeStation, Network: "home", Device: "wlan0", Addressing: manifest.AddressingDHCP, Roaming: &manifest.Roaming{Media: []string{"rf24"}}}},
	})
	slot, first, next, strong, weak := 0, 0, 1000, 35, -100
	initial := manifest.VAPReference{Service: "gateway", Replica: 1, Radio: "ap", Slot: &slot}
	final := manifest.VAPReference{Service: "gateway", Replica: 1, Radio: "other-ap", Slot: &slot}
	doc.Spec.WirelessScenarios = []manifest.WirelessScenario{{
		Name: "roam", Station: manifest.RadioReference{Service: "station", Replica: 1, Radio: "station"},
		APs:        []manifest.VAPReference{initial, final},
		Steps:      []manifest.RFStep{{AtMs: &first, AP: initial, SNRDb: &strong}, {AtMs: &next, AP: initial, SNRDb: &weak}, {AtMs: &next, AP: final, SNRDb: &strong}},
		Assertions: manifest.RoamAssertions{InitialAP: initial, FinalAP: final, SameIPv4: true, MaxRoamMs: 5000, MaxGapMs: 3000},
	}}
	if err := manifest.Validate(doc); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDesiredSnapshot("other", data); err != nil {
		t.Fatal(err)
	}
	group, err := store.AllocateWirelessGroup("other")
	if err != nil {
		t.Fatal(err)
	}
	active, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	var owned []persist.WirelessRadio
	for _, service := range active.Services {
		for _, instance := range service.Instances {
			for _, radio := range instance.Radios {
				owned = append(owned, persist.WirelessRadio{Deployment: "other", Service: service.Name, LogicalName: radio.Name, ManagerName: radio.ManagerName, GroupBit: group.Bit})
			}
		}
	}
	if err := store.ReplaceWirelessRadios("other", owned); err != nil {
		t.Fatal(err)
	}
	legacy := plan.Deployment{Name: "legacy", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:00:aa:01"}}}}}}}
	installed := false
	var commands []string
	factory := func(context.Context) (*hwsim.Client, error) {
		return hwsim.NewClient(rfHealthTransport{installed: &installed, commands: &commands}), nil
	}
	installer := rfBaselineInstallerFunc(func(_ context.Context, baseline string) error {
		if !strings.Contains(baseline, "02:00:00:00:aa:01") || !strings.Contains(baseline, active.WirelessScenarios[0].Station.MAC) {
			t.Fatalf("global baseline omitted a deployment: %s", baseline)
		}
		installed = true
		return nil
	})
	if err := preflightWirelessForApply(context.Background(), store, manifest.Document{}, legacy, factory, installer); err != nil {
		t.Fatal(err)
	}
	if !installed || !strings.Contains(strings.Join(commands, ","), "medium-health") {
		t.Fatalf("active RF scenario lost during legacy apply: installed=%t, commands=%v", installed, commands)
	}
}

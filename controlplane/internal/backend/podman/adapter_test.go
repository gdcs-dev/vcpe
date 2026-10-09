package podman

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
)

func TestContainerPIDInspectsRunningContainer(t *testing.T) {
	var gotName string
	var gotArgs []string
	adapter := &Adapter{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return []byte(`{"Running":true,"Pid":4812}`), nil
	}}
	pid, err := adapter.ContainerPID(context.Background(), "vcpe-edge-gateway-1")
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"inspect", "--type", "container", "--format", "{{json .State}}", "vcpe-edge-gateway-1"}
	if pid != 4812 || gotName != "podman" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("pid/command = %d, %q %#v", pid, gotName, gotArgs)
	}
}

func TestContainerPIDRejectsUnreadyContainer(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "stopped", json: `{"Running":false,"Pid":0}`, want: "not running"},
		{name: "missing pid", json: `{"Running":true,"Pid":0}`, want: "invalid VM-side PID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &Adapter{run: func(context.Context, string, ...string) ([]byte, error) {
				return []byte(test.json), nil
			}}
			_, err := adapter.ContainerPID(context.Background(), "vcpe-edge-gateway-1")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestContainerPIDReportsInspectFailure(t *testing.T) {
	adapter := &Adapter{run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("no such container"), fmt.Errorf("exit status 125")
	}}
	_, err := adapter.ContainerPID(context.Background(), "vcpe-missing")
	if err == nil || !strings.Contains(err.Error(), "no such container") {
		t.Fatalf("error = %v", err)
	}
}

func TestContainerFileReadsRuntimeContractWithoutLeakingFailureOutput(t *testing.T) {
	var gotArgs []string
	adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte("state=ready\n"), nil
	}}
	contents, err := adapter.ContainerFile(context.Background(), "wireless-client-1", "/run/vcpe/wireless-client/status")
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"exec", "wireless-client-1", "cat", "/run/vcpe/wireless-client/status"}
	if string(contents) != "state=ready\n" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("contents/command = %q %#v", contents, gotArgs)
	}

	adapter.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("SENTINEL-runtime-output"), errors.New("exit status 1")
	}
	_, err = adapter.ContainerFile(context.Background(), "wireless-client-1", "/run/vcpe/wireless-client/status")
	if err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatalf("bounded read error = %v", err)
	}
}

func TestContainerVAPStatusUsesFixedProbeAndRetainsNonSecretFailureDetails(t *testing.T) {
	var gotArgs []string
	adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte(`{"radio":"ap5","slot":7,"enabled":false}`), errors.New("exit status 1")
	}}
	status, err := adapter.ContainerVAPStatus(context.Background(), "edge-gateway-1")
	if err == nil || strings.Contains(err.Error(), "ap5") || string(status) == "" {
		t.Fatalf("status = %q, error = %v", status, err)
	}
	if want := []string{"exec", "edge-gateway-1", "gateway-health-probe", "vaps"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("probe arguments = %v, want %v", gotArgs, want)
	}
}

func TestContainerMeshStatusUsesFixedProbe(t *testing.T) {
	var gotArgs []string
	adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte(`{"radio":"backhaul","peers":1}`), nil
	}}
	if _, err := adapter.ContainerMeshStatus(context.Background(), "edge-gateway-1"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"exec", "edge-gateway-1", "gateway-health-probe", "mesh"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("probe arguments = %v, want %v", gotArgs, want)
	}
}

func TestProbeLANUsesWirelessDeviceAndHidesFailureOutput(t *testing.T) {
	var gotArgs []string
	adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte("SENTINEL-runtime-output"), errors.New("exit status 1")
	}}
	err := adapter.ProbeLAN(context.Background(), "edge-remote-probe-1", "wlan0", "10.0.0.1")
	if err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatalf("probe error = %v", err)
	}
	want := []string{"exec", "edge-remote-probe-1", "ping", "-n", "-c", "1", "-W", "1", "-I", "wlan0", "10.0.0.1"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("probe command = %v, want %v", gotArgs, want)
	}
}

func TestObserveStationReadsLiveAuthenticationLinkAndIPv4(t *testing.T) {
	var commands [][]string
	adapter := &Adapter{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		switch args[2] {
		case "wpa_cli":
			return []byte("wpa_state=COMPLETED\nbssid=02:00:00:00:00:01\n"), nil
		case "iw":
			return []byte("Connected to 02:00:00:00:00:01 (on wlan0)\n"), nil
		default:
			return []byte(`[{"addr_info":[{"local":"10.0.0.10","prefixlen":24}]}]`), nil
		}
	}}
	got, err := adapter.ObserveStation(context.Background(), "client-1", "wlan0")
	if err != nil || !got.Authenticated || got.BSSID != "02:00:00:00:00:01" || got.IPv4.String() != "10.0.0.10/24" {
		t.Fatalf("station observation = %+v, error = %v", got, err)
	}
	want := [][]string{
		{"podman", "exec", "client-1", "wpa_cli", "-i", "wlan0", "status"},
		{"podman", "exec", "client-1", "iw", "dev", "wlan0", "link"},
		{"podman", "exec", "client-1", "ip", "-j", "-4", "addr", "show", "dev", "wlan0"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestObserveStationDoesNotConfirmTransitionalLink(t *testing.T) {
	for _, link := range []string{"Connected to 02:00:00:00:00:02 (on wlan0)", "Not connected."} {
		adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			switch args[2] {
			case "wpa_cli":
				return []byte("wpa_state=COMPLETED\nbssid=02:00:00:00:00:01\n"), nil
			case "iw":
				return []byte(link), nil
			default:
				return []byte(`[{"addr_info":[{"local":"10.0.0.10","prefixlen":24}]}]`), nil
			}
		}}
		observation, err := adapter.ObserveStation(context.Background(), "client-1", "wlan0")
		if err != nil || observation.Authenticated || observation.BSSID != "" || !observation.IPv4.IsValid() {
			t.Fatalf("transitional link %q: %+v, error = %v", link, observation, err)
		}
	}
}

func TestVerifyMeshGatewayImageRequiresMatchingDigestAndCapability(t *testing.T) {
	const reference = "ghcr.io/gdcs-dev/gateway@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const localReference = "ghcr.io/gdcs-dev/gateway:dev"
	for _, test := range []struct {
		name, reference, output string
		valid                   bool
	}{
		{"verified", reference, `[{"RepoDigests":["` + reference + `"],"Labels":{"org.gdcs-dev.vcpe.gateway.mesh-sae":"1"}}]`, true},
		{"missing label", reference, `[{"RepoDigests":["` + reference + `"],"Labels":{}}]`, false},
		{"wrong digest", reference, `[{"RepoDigests":["ghcr.io/gdcs-dev/gateway@sha256:other"],"Labels":{"org.gdcs-dev.vcpe.gateway.mesh-sae":"1"}}]`, false},
		{"local verified", localReference, `[{"RepoTags":["` + localReference + `"],"Labels":{"org.gdcs-dev.vcpe.gateway.mesh-sae":"1"}}]`, true},
		{"local missing label", localReference, `[{"RepoTags":["` + localReference + `"],"Labels":{}}]`, false},
		{"local wrong tag", localReference, `[{"RepoTags":["ghcr.io/gdcs-dev/gateway:other"],"Labels":{"org.gdcs-dev.vcpe.gateway.mesh-sae":"1"}}]`, false},
		{"malformed", reference, `not json`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var gotArgs []string
			adapter := &Adapter{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				gotArgs = append([]string(nil), args...)
				return []byte(test.output), nil
			}}
			err := adapter.VerifyMeshGatewayImage(context.Background(), test.reference)
			if test.valid && err != nil || !test.valid && err == nil {
				t.Fatalf("image verification = %v, expected valid = %t", err, test.valid)
			}
			if want := []string{"image", "inspect", test.reference}; !reflect.DeepEqual(gotArgs, want) {
				t.Fatalf("inspect command = %v, want %v", gotArgs, want)
			}
		})
	}
}

func TestImageCommandArgs(t *testing.T) {
	args, err := buildImageArgs(image.BuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", File: "services/bng/Containerfile"})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if !reflect.DeepEqual(args, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "-f", "services/bng/Containerfile", "services/bng"}) {
		t.Fatalf("unexpected build args: %#v", args)
	}

	noCacheArgs, err := buildImageArgs(image.BuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", NoCache: true})
	if err != nil {
		t.Fatalf("build args no-cache: %v", err)
	}
	if !reflect.DeepEqual(noCacheArgs, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "--no-cache", "services/bng"}) {
		t.Fatalf("unexpected no-cache build args: %#v", noCacheArgs)
	}

	// Single platform: regular tagged-image mode
	singlePlatform, err := buildImageArgs(image.BuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/amd64"}})
	if err != nil {
		t.Fatalf("build args single platform: %v", err)
	}
	if !reflect.DeepEqual(singlePlatform, []string{"build", "--platform", "linux/amd64", "-t", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected single-platform build args: %#v", singlePlatform)
	}

	// Multi-platform: --manifest mode with comma-joined platforms
	multiPlatform, err := buildImageArgs(image.BuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/amd64", "linux/arm64"}})
	if err != nil {
		t.Fatalf("build args multi platform: %v", err)
	}
	if !reflect.DeepEqual(multiPlatform, []string{"build", "--platform", "linux/amd64,linux/arm64", "--manifest", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected multi-platform build args: %#v", multiPlatform)
	}

	pull, err := pullImageArgs(image.PullRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("pull args: %v", err)
	}
	if !reflect.DeepEqual(pull, []string{"pull", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected pull args: %#v", pull)
	}

	push, err := pushImageArgs(image.PushRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("push args: %v", err)
	}
	if !reflect.DeepEqual(push, []string{"push", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected push args: %#v", push)
	}

	tag, err := tagImageArgs(image.TagRequest{Source: "ghcr.io/gdcs-dev/bng:dev", Target: "localhost/bng:test"})
	if err != nil {
		t.Fatalf("tag args: %v", err)
	}
	if !reflect.DeepEqual(tag, []string{"tag", "ghcr.io/gdcs-dev/bng:dev", "localhost/bng:test"}) {
		t.Fatalf("unexpected tag args: %#v", tag)
	}
}

func TestImageCommandArgsValidation(t *testing.T) {
	if _, err := buildImageArgs(image.BuildRequest{Tags: []string{"x"}}); err == nil {
		t.Fatalf("expected build context validation failure")
	}
	if _, err := pullImageArgs(image.PullRequest{}); err == nil {
		t.Fatalf("expected pull validation failure")
	}
	if _, err := pushImageArgs(image.PushRequest{}); err == nil {
		t.Fatalf("expected push validation failure")
	}
	if _, err := tagImageArgs(image.TagRequest{Source: "x"}); err == nil {
		t.Fatalf("expected tag validation failure")
	}
}

func TestNetworkArgs(t *testing.T) {
	// No driver (bridge default) — unchanged legacy behavior
	args := buildNetworkArgs(NetworkSpec{Name: "example-wan", Subnet: "10.7.200.0/24", HostGateway: "10.7.200.254"})
	if !reflect.DeepEqual(args, []string{"network", "create", "--subnet", "10.7.200.0/24", "--gateway", "10.7.200.254", "example-wan"}) {
		t.Fatalf("unexpected bridge args: %#v", args)
	}

	// macvlan with parent option
	macvlan := buildNetworkArgs(NetworkSpec{
		Name:          "example-wan",
		Driver:        "macvlan",
		DriverOptions: map[string]string{"parent": "eth0"},
		Subnet:        "192.168.1.0/24",
		HostGateway:   "192.168.1.1",
	})
	if !reflect.DeepEqual(macvlan, []string{"network", "create", "--driver", "macvlan", "-o", "parent=eth0", "--subnet", "192.168.1.0/24", "--gateway", "192.168.1.1", "example-wan"}) {
		t.Fatalf("unexpected macvlan args: %#v", macvlan)
	}

	// ipvlan with multiple sorted options
	ipvlan := buildNetworkArgs(NetworkSpec{
		Name:          "example-up",
		Driver:        "ipvlan",
		DriverOptions: map[string]string{"parent": "eth1", "mode": "l2"},
		Subnet:        "10.0.0.0/24",
	})
	if !reflect.DeepEqual(ipvlan, []string{"network", "create", "--driver", "ipvlan", "-o", "mode=l2", "-o", "parent=eth1", "--subnet", "10.0.0.0/24", "example-up"}) {
		t.Fatalf("unexpected ipvlan args: %#v", ipvlan)
	}

	// Custom ipam-driver (non-none): subnet IS included
	withIPAM := buildNetworkArgs(NetworkSpec{Name: "custom", IPAMDriver: "host-local", Subnet: "10.1.0.0/24"})
	if !reflect.DeepEqual(withIPAM, []string{"network", "create", "--ipam-driver", "host-local", "--subnet", "10.1.0.0/24", "custom"}) {
		t.Fatalf("unexpected ipam-driver args: %#v", withIPAM)
	}

	// ipam-driver=none: subnet is NOT included (Podman rejects subnet with none driver)
	noneIPAM := buildNetworkArgs(NetworkSpec{Name: "example-wan", IPAMDriver: "none", Subnet: "10.7.200.0/24"})
	if !reflect.DeepEqual(noneIPAM, []string{"network", "create", "--ipam-driver", "none", "example-wan"}) {
		t.Fatalf("unexpected none-ipam args: %#v", noneIPAM)
	}
}

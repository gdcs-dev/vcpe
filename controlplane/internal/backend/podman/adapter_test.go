package podman

import (
	"reflect"
	"testing"
)

func TestImageCommandArgs(t *testing.T) {
	args, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", File: "services/bng/Containerfile"})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if !reflect.DeepEqual(args, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "-f", "services/bng/Containerfile", "services/bng"}) {
		t.Fatalf("unexpected build args: %#v", args)
	}

	noCacheArgs, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", NoCache: true})
	if err != nil {
		t.Fatalf("build args no-cache: %v", err)
	}
	if !reflect.DeepEqual(noCacheArgs, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "--no-cache", "services/bng"}) {
		t.Fatalf("unexpected no-cache build args: %#v", noCacheArgs)
	}

	// Single platform: --manifest mode
	singlePlatform, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/amd64"}})
	if err != nil {
		t.Fatalf("build args single platform: %v", err)
	}
	if !reflect.DeepEqual(singlePlatform, []string{"build", "--platform", "linux/amd64", "--manifest", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected single-platform build args: %#v", singlePlatform)
	}

	// Multi-platform: --manifest mode with comma-joined platforms
	multiPlatform, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/amd64", "linux/arm64"}})
	if err != nil {
		t.Fatalf("build args multi platform: %v", err)
	}
	if !reflect.DeepEqual(multiPlatform, []string{"build", "--platform", "linux/amd64,linux/arm64", "--manifest", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected multi-platform build args: %#v", multiPlatform)
	}

	pull, err := pullImageArgs(ImagePullRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("pull args: %v", err)
	}
	if !reflect.DeepEqual(pull, []string{"pull", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected pull args: %#v", pull)
	}

	push, err := pushImageArgs(ImagePushRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("push args: %v", err)
	}
	if !reflect.DeepEqual(push, []string{"push", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected push args: %#v", push)
	}

	tag, err := tagImageArgs(ImageTagRequest{Source: "ghcr.io/gdcs-dev/bng:dev", Target: "localhost/bng:test"})
	if err != nil {
		t.Fatalf("tag args: %v", err)
	}
	if !reflect.DeepEqual(tag, []string{"tag", "ghcr.io/gdcs-dev/bng:dev", "localhost/bng:test"}) {
		t.Fatalf("unexpected tag args: %#v", tag)
	}
}

func TestImageCommandArgsValidation(t *testing.T) {
	if _, err := buildImageArgs(ImageBuildRequest{Tags: []string{"x"}}); err == nil {
		t.Fatalf("expected build context validation failure")
	}
	if _, err := pullImageArgs(ImagePullRequest{}); err == nil {
		t.Fatalf("expected pull validation failure")
	}
	if _, err := pushImageArgs(ImagePushRequest{}); err == nil {
		t.Fatalf("expected push validation failure")
	}
	if _, err := tagImageArgs(ImageTagRequest{Source: "x"}); err == nil {
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

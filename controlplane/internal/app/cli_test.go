package app

import (
	"strings"
	"testing"
)

func TestParsePublicCommands(t *testing.T) {
	cases := map[string][]string{
		"init":   {"init"},
		"build":  {"build", "--manifest", "m.yaml"},
		"up":     {"up", "--manifest", "m.yaml"},
		"apply":  {"apply", "--manifest", "m.yaml"},
		"plan":   {"plan", "--manifest", "m.yaml"},
		"status": {"status"},
		"logs":   {"logs"},
		"config": {"config", "show"},
		"state":  {"state", "reset"},
	}
	for command, args := range cases {
		t.Run(command, func(t *testing.T) {
			opts, err := parseArgs("vcpe", args)
			if err != nil {
				t.Fatalf("parse %s: %v", command, err)
			}
			if opts.Command != command {
				t.Fatalf("expected command %q, got %q", command, opts.Command)
			}
		})
	}
}

func TestParseNameSelector(t *testing.T) {
	opts, err := parseArgs("vcpe", []string{"status", "--name", "edge"})
	if err != nil {
		t.Fatalf("parse status --name: %v", err)
	}
	if opts.Name != "edge" {
		t.Fatalf("expected name edge, got %q", opts.Name)
	}
}

func TestDownRequiresName(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"down"})
	if err == nil || !strings.Contains(err.Error(), "requires --name") {
		t.Fatalf("expected down to require --name, got %v", err)
	}

	opts, err := parseArgs("vcpe", []string{"down", "--name", "edge"})
	if err != nil {
		t.Fatalf("parse down --name: %v", err)
	}
	if opts.Name != "edge" {
		t.Fatalf("expected name edge, got %q", opts.Name)
	}
}

func TestDestroyRequiresForce(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"destroy", "--name", "edge"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected destroy to require --force, got %v", err)
	}
	if _, err := parseArgs("vcpe", []string{"destroy", "--name", "edge", "--force"}); err != nil {
		t.Fatalf("destroy --name --force should parse: %v", err)
	}
}

func TestBuildNoCacheAccepted(t *testing.T) {
	opts, err := parseArgs("vcpe", []string{"build", "--manifest", "m.yaml", "--no-cache"})
	if err != nil {
		t.Fatalf("build --no-cache: %v", err)
	}
	if !opts.NoCache {
		t.Fatal("expected NoCache set")
	}
}

func TestNoCacheRejectedForNonBuild(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"up", "--manifest", "m.yaml", "--no-cache"})
	if err == nil || !strings.Contains(err.Error(), "only supported for build") {
		t.Fatalf("expected non-build --no-cache rejection, got %v", err)
	}
}

func TestUpRequiresManifest(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"up"})
	if err == nil || !strings.Contains(err.Error(), "requires --manifest") {
		t.Fatalf("expected up to require --manifest, got %v", err)
	}
}

func TestServiceGrammar(t *testing.T) {
	opts, err := parseArgs("vcpe", []string{"service", "bng", "status"})
	if err != nil {
		t.Fatalf("service bng status: %v", err)
	}
	if opts.Command != "service" || opts.CommandArgs[0] != "bng" || opts.CommandArgs[1] != "status" {
		t.Fatalf("unexpected service parse: %+v", opts)
	}

	_, err = parseArgs("vcpe", []string{"service", "unknown", "status"})
	if err == nil || !strings.Contains(err.Error(), "unsupported service") {
		t.Fatalf("expected unsupported service error, got %v", err)
	}

	_, err = parseArgs("vcpe", []string{"service", "bng", "down"})
	if err == nil || !strings.Contains(err.Error(), "requires --name") {
		t.Fatalf("expected service down to require --name, got %v", err)
	}

	_, err = parseArgs("vcpe", []string{"service", "bng", "down", "--name", "edge"})
	if err == nil || !strings.Contains(err.Error(), "requires --force") {
		t.Fatalf("expected service down to require --force, got %v", err)
	}
}

func TestRetiredWrapperHints(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"bng", "status"})
	if err == nil || !strings.Contains(err.Error(), "vcpe service bng") {
		t.Fatalf("expected bng wrapper hint, got %v", err)
	}

	_, err = parseArgs("vcpe", []string{"net", "verify"})
	if err == nil || !strings.Contains(err.Error(), "vcpe up (apply) and vcpe status") {
		t.Fatalf("expected net migration hint, got %v", err)
	}
}

func TestUnknownCommandRejected(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

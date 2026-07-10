package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempManifest creates a temporary manifest file and returns its path.
func tempManifest(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(f, "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: test\nspec:\n  networks: []\n  services: []\n")
	_ = f.Close()
	// Return absolute path with dir separator so resolveManifestPath treats it as a path
	abs, _ := filepath.Abs(f.Name())
	return abs
}

func TestParsePublicCommands(t *testing.T) {
	m := tempManifest(t)
	cases := map[string][]string{
		"init":   {"init"},
		"build":  {"build", "--manifest", m},
		"up":     {"up", "--manifest", m},
		"apply":  {"apply", "--manifest", m},
		"plan":   {"plan", "--manifest", m},
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

func TestDownNameOptional(t *testing.T) {
	// --name is now optional for down; parseArgs should accept it without --name.
	opts, err := parseArgs("vcpe", []string{"down"})
	if err != nil {
		t.Fatalf("parse down without --name: %v", err)
	}
	if opts.Name != "" {
		t.Fatalf("expected empty name, got %q", opts.Name)
	}

	opts, err = parseArgs("vcpe", []string{"down", "--name", "edge"})
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
	m := tempManifest(t)
	opts, err := parseArgs("vcpe", []string{"build", "--manifest", m, "--no-cache"})
	if err != nil {
		t.Fatalf("build --no-cache: %v", err)
	}
	if !opts.NoCache {
		t.Fatal("expected NoCache set")
	}
}

func TestNoCacheRejectedForNonBuild(t *testing.T) {
	m := tempManifest(t)
	_, err := parseArgs("vcpe", []string{"up", "--manifest", m, "--no-cache"})
	if err == nil || !strings.Contains(err.Error(), "only supported for build") {
		t.Fatalf("expected non-build --no-cache rejection, got %v", err)
	}
}

func TestUpRequiresManifest(t *testing.T) {
	// With no manifests discoverable, up without --manifest should error
	// with a helpful message pointing to `vcpe manifest list`
	_, err := parseArgs("vcpe", []string{"up"})
	if err == nil {
		t.Fatal("expected up without --manifest to error")
	}
	// Accept either the old message or the new discovery message
	if !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("expected manifest-related error, got %v", err)
	}
}

func TestRetiredWrapperHints(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"bng", "status"})
	if err == nil || !strings.Contains(err.Error(), "vcpe up --manifest") {
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

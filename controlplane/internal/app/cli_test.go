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
		"init":     {"init"},
		"build":    {"build", "--manifest", m},
		"up":       {"up", "--manifest", m},
		"apply":    {"apply", "--manifest", m},
		"plan":     {"plan", "--manifest", m},
		"status":   {"status"},
		"diagnose": {"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa", "--client-service", "config"},
		"logs":     {"logs"},
		"config":   {"config", "show"},
		"state":    {"state", "reset"},
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

func TestParseDiagnose(t *testing.T) {
	opts, err := parseArgs("vcpe", []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa", "--client-service", "config", "--replica", "1", "--json"})
	if err != nil {
		t.Fatalf("parse diagnose: %v", err)
	}
	if opts.Name != "edge" || opts.From != "gateway" || opts.To != "webpa" || opts.ClientService != "config" || opts.Replica == nil || *opts.Replica != 1 || !opts.OutputJSON {
		t.Fatalf("options = %+v", opts)
	}
}

func TestParseWebhookDiagnose(t *testing.T) {
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "passive", args: []string{"diagnose", "--name", "edge", "--from", "event-sink", "--to", "webhook"}},
		{name: "active", args: []string{"diagnose", "--name", "edge", "--from", "event-sink", "--to", "webhook", "--allow-active-callback", "--event", "devices/diagnostic", "--device-id", "mac:001122334455"}},
		{name: "client service rejected", args: []string{"diagnose", "--name", "edge", "--from", "event-sink", "--to", "webhook", "--client-service", "config"}, want: "only for --to webpa"},
		{name: "active input without consent", args: []string{"diagnose", "--name", "edge", "--from", "event-sink", "--to", "webhook", "--event", "devices/diagnostic"}, want: "require active callback consent"},
		{name: "webpa webhook flag rejected", args: []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa", "--client-service", "config", "--allow-active-callback"}, want: "webhook invocation fields"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			opts, err := parseArgs("vcpe", testCase.args)
			if testCase.want != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.want) {
					t.Fatalf("parse error = %v, want %q", err, testCase.want)
				}
				return
			}
			if err != nil || opts.To != "webhook" {
				t.Fatalf("options = %+v, error = %v", opts, err)
			}
		})
	}
}

func TestParseCallbackDiagnose(t *testing.T) {
	valid := []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "callback", "--client-service", "apparmor-simulator", "--subscriber", "event-sink", "--subscriber-replica", "1", "--allow-active-event", "--event", "devices/diagnostic", "--device-id", "mac:001122334455", "--json"}
	opts, err := parseArgs("vcpe", valid)
	if err != nil {
		t.Fatalf("parse callback diagnose: %v", err)
	}
	if opts.Subscriber != "event-sink" || opts.SubscriberReplica == nil || *opts.SubscriberReplica != 1 || !opts.AllowActiveEvent || !opts.OutputJSON {
		t.Fatalf("callback options = %+v", opts)
	}
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing consent", args: []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "callback", "--client-service", "apparmor-simulator", "--subscriber", "event-sink", "--event", "devices/diagnostic", "--device-id", "mac:001122334455"}, want: "active event consent"},
		{name: "missing subscriber", args: []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "callback", "--client-service", "apparmor-simulator", "--allow-active-event", "--event", "devices/diagnostic", "--device-id", "mac:001122334455"}, want: "--subscriber"},
		{name: "webhook flag", args: []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "callback", "--client-service", "apparmor-simulator", "--subscriber", "event-sink", "--allow-active-event", "--allow-active-callback", "--event", "devices/diagnostic", "--device-id", "mac:001122334455"}, want: "webhook invocation fields"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := parseArgs("vcpe", testCase.args); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parse error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestDiagnoseRequiresSelectors(t *testing.T) {
	_, err := parseArgs("vcpe", []string{"diagnose", "--name", "edge"})
	if err == nil || !strings.Contains(err.Error(), "requires --from") {
		t.Fatalf("expected selector error, got %v", err)
	}
	_, err = parseArgs("vcpe", []string{"diagnose", "--from", "gateway", "--to", "webpa", "--client-service", "config"})
	if err != nil {
		t.Fatalf("diagnose without name: %v", err)
	}
	_, err = parseArgs("vcpe", []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa"})
	if err == nil || !strings.Contains(err.Error(), "--client-service") {
		t.Fatalf("expected required client-service error, got %v", err)
	}
	_, err = parseArgs("vcpe", []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa", "--replica", "bad"})
	if err == nil || !strings.Contains(err.Error(), "non-negative integer") {
		t.Fatalf("expected replica error, got %v", err)
	}
	_, err = parseArgs("vcpe", []string{"diagnose", "--name", "edge", "--from", "gateway", "--to", "webpa", "--client-service", "../config"})
	if err == nil || !strings.Contains(err.Error(), "invalid --client-service") {
		t.Fatalf("expected client-service error, got %v", err)
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

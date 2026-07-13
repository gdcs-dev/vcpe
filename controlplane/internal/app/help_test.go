package app

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update is the flag for regenerating golden files.
// Run: go test ./internal/app/ -run TestHelp -update
var update = flag.Bool("update", false, "update golden files")

// checkGolden compares got against the committed golden file for name. When
// -update is set it writes the golden file instead of comparing.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "help", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Fatalf("golden file %s not found; run: go test ./internal/app/ -run TestHelp -update", path)
	}
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, string(want))
	}
}

// TestHelpCoverage asserts that every key in topLevelCommands (except aliases
// and the hidden daemon command) has a corresponding commandHelp entry.
func TestHelpCoverage(t *testing.T) {
	// These commands are intentionally absent from commandHelp.
	excluded := map[string]struct{}{
		"apply":   {}, // alias for up
		"destroy": {}, // alias for down
		"daemon":  {}, // hidden
	}
	for cmd := range topLevelCommands {
		if _, skip := excluded[cmd]; skip {
			continue
		}
		if _, ok := commandHelp[cmd]; !ok {
			t.Errorf("topLevelCommands contains %q but commandHelp has no entry for it", cmd)
		}
	}
}

func TestHelpGlobal(t *testing.T) {
	got := GlobalHelp()
	checkGolden(t, "global", got)
}

func TestHelpForInit(t *testing.T) {
	got := HelpFor("init")
	checkGolden(t, "init", got)
}

func TestHelpForBuild(t *testing.T) {
	got := HelpFor("build")
	checkGolden(t, "build", got)
}

func TestHelpForPush(t *testing.T) {
	got := HelpFor("push")
	checkGolden(t, "push", got)
}

func TestHelpForUp(t *testing.T) {
	got := HelpFor("up")
	checkGolden(t, "up", got)
}

func TestHelpForPlan(t *testing.T) {
	got := HelpFor("plan")
	checkGolden(t, "plan", got)
}

func TestHelpForDown(t *testing.T) {
	got := HelpFor("down")
	checkGolden(t, "down", got)
}

func TestHelpForStatus(t *testing.T) {
	got := HelpFor("status")
	checkGolden(t, "status", got)
}

func TestHelpForLogs(t *testing.T) {
	got := HelpFor("logs")
	checkGolden(t, "logs", got)
}

func TestHelpForConfig(t *testing.T) {
	got := HelpFor("config")
	checkGolden(t, "config", got)
}

func TestHelpForState(t *testing.T) {
	got := HelpFor("state")
	checkGolden(t, "state", got)
}

// TestHelpAliasRedirects asserts aliases produce redirect messages.
func TestHelpAliasRedirects(t *testing.T) {
	apply := HelpFor("apply")
	if !strings.Contains(apply, "alias for up") {
		t.Errorf("HelpFor(apply) expected 'alias for up', got: %s", apply)
	}
	destroy := HelpFor("destroy")
	if !strings.Contains(destroy, "alias for down") {
		t.Errorf("HelpFor(destroy) expected 'alias for down', got: %s", destroy)
	}
}

// TestHelpFlagExitsZero asserts that passing --help returns nil (exit 0).
// Content correctness is covered by the golden-file tests above.
func TestHelpFlagExitsZero(t *testing.T) {
	if err := ExecuteCLI("vcpe", []string{"--help"}); err != nil {
		t.Errorf("vcpe --help: expected nil, got %v", err)
	}
}

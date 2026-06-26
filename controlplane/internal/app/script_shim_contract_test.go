package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScriptsVcpeShimIsRetired(t *testing.T) {
	repoRoot := repoRoot(t)
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "vcpe"), "status")
	cmd.Env = baseEnv()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected scripts/vcpe to fail")
	}
	if !strings.Contains(err.Error(), "exit status 2") {
		t.Fatalf("expected exit status 2, got %v (%s)", err, strings.TrimSpace(string(out)))
	}
	text := string(out)
	if !strings.Contains(text, "scripts/vcpe has been retired") {
		t.Fatalf("expected retirement message, got %q", text)
	}
	if !strings.Contains(text, "use: vcpe status") {
		t.Fatalf("expected canonical command mapping, got %q", text)
	}
}

func TestServiceAndNetScriptShimsAreRetired(t *testing.T) {
	repoRoot := repoRoot(t)
	cases := []struct {
		script      string
		args        []string
		expectUsage string
	}{
		{script: "bng", args: []string{"up"}, expectUsage: "use: vcpe service bng up"},
		{script: "gateway", args: []string{"status"}, expectUsage: "use: vcpe service gateway status"},
		{script: "routerd", args: []string{"down"}, expectUsage: "use: vcpe service routerd down"},
		{script: "webpa", args: []string{"build"}, expectUsage: "use: vcpe service webpa build"},
		{script: "xb10", args: []string{"logs"}, expectUsage: "use: vcpe service xb10 logs"},
		{script: "client", args: []string{"status"}, expectUsage: "use: vcpe service client status"},
		{script: "net", args: []string{"verify"}, expectUsage: "use: vcpe up (apply) and vcpe status for verification"},
	}

	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			scriptPath := filepath.Join(repoRoot, "scripts", tc.script)
			cmd := exec.Command(scriptPath, tc.args...)
			cmd.Env = baseEnv()
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected retired script to fail")
			}
			if !strings.Contains(err.Error(), "exit status 2") {
				t.Fatalf("expected exit status 2, got %v (%s)", err, strings.TrimSpace(string(out)))
			}
			if !strings.Contains(string(out), tc.expectUsage) {
				t.Fatalf("expected replacement usage %q, got %q", tc.expectUsage, string(out))
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func baseEnv() []string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.TempDir()
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
	}
	return env
}

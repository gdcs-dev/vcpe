package hwsim

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCommandTransportMapping(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		wantName   string
		wantPrefix []string
	}{
		{
			name:       "macOS",
			goos:       "darwin",
			wantName:   "podman",
			wantPrefix: []string{"machine", "ssh", "--", "sudo", managerPath},
		},
		{name: "Linux", goos: "linux", wantName: managerPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			runner := func(_ context.Context, name string, args ...string) (CommandResult, error) {
				gotName = name
				gotArgs = append([]string(nil), args...)
				return CommandResult{}, nil
			}
			transport, err := newCommandTransport(test.goos, 0, false, false, true, true, runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.Execute(context.Background(), "version"); err != nil {
				t.Fatal(err)
			}
			wantArgs := append(append([]string(nil), test.wantPrefix...), "version")
			if gotName != test.wantName || !reflect.DeepEqual(gotArgs, wantArgs) {
				t.Fatalf("command = %q %#v, want %q %#v", gotName, gotArgs, test.wantName, wantArgs)
			}
		})
	}
}

func TestCommandTransportRejectsUnsupportedEnvironments(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		euid       int
		wsl        bool
		rootless   bool
		hasPodman  bool
		hasManager bool
		want       string
	}{
		{name: "WSL", goos: "linux", wsl: true, hasPodman: true, hasManager: true, want: "WSL"},
		{name: "rootless", goos: "linux", rootless: true, hasPodman: true, hasManager: true, want: "rootful"},
		{name: "unprivileged", goos: "linux", euid: 501, hasPodman: true, hasManager: true, want: "root privileges"},
		{name: "missing Linux manager", goos: "linux", hasPodman: true, want: "not installed"},
		{name: "missing macOS podman", goos: "darwin", hasManager: true, want: "requires podman"},
		{name: "unsupported", goos: "windows", hasPodman: true, hasManager: true, want: "not supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			runner := func(context.Context, string, ...string) (CommandResult, error) {
				called = true
				return CommandResult{}, nil
			}
			_, err := newCommandTransport(test.goos, test.euid, test.wsl, test.rootless, test.hasPodman, test.hasManager, runner)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if called {
				t.Fatal("runner called for rejected environment")
			}
		})
	}
}

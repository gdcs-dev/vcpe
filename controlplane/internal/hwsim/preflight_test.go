package hwsim

import (
	"context"
	"strings"
	"testing"
)

func TestPreflightSkipsManagerWithoutRadios(t *testing.T) {
	called := false
	err := Preflight(context.Background(), 0, func(context.Context) (*Client, error) {
		called = true
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("client factory called for deployment without radios")
	}
}

func TestPreflightChecksVersionAndDoctor(t *testing.T) {
	var commands []string
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		commands = append(commands, args[0])
		switch args[0] {
		case "version":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"test"}}`)}, nil
		case "doctor":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"checks":[]}}`)}, nil
		default:
			t.Fatalf("unexpected command %q", args[0])
			return CommandResult{}, nil
		}
	}))
	err := Preflight(context.Background(), 1, func(context.Context) (*Client, error) { return client, nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "version,doctor" {
		t.Fatalf("commands = %v", commands)
	}
}

func TestPreflightRejectsMissingMeshCapability(t *testing.T) {
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		switch args[0] {
		case "version":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"old"}}`)}, nil
		case "list":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":[]}`)}, nil
		case "doctor":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"checks":[]}}`)}, nil
		default:
			t.Fatalf("unexpected command %q", args[0])
			return CommandResult{}, nil
		}
	}))
	err := PreflightFeatures(context.Background(), 1, true, func(context.Context) (*Client, error) { return client, nil })
	if err == nil || !strings.Contains(err.Error(), "mesh") {
		t.Fatalf("error = %v, want missing mesh capability", err)
	}
}

func TestPreflightRequiresProbedMeshCapability(t *testing.T) {
	var commands []string
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		commands = append(commands, strings.Join(args, " "))
		switch args[0] {
		case "version":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"mesh"}}`)}, nil
		case "list":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":[]}`)}, nil
		case "doctor":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"meshCapability":"supported","checks":[]}}`)}, nil
		default:
			t.Fatalf("unexpected command %q", args[0])
			return CommandResult{}, nil
		}
	}))
	if err := PreflightFeatures(context.Background(), 1, true, func(context.Context) (*Client, error) { return client, nil }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "version,list,doctor --probe" {
		t.Fatalf("commands = %v, want a probed doctor", commands)
	}
}

func TestPreflightUsesLiveReadyMeshWithoutAllocatingProbe(t *testing.T) {
	var commands []string
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		commands = append(commands, strings.Join(args, " "))
		switch args[0] {
		case "version":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"mesh"}}`)}, nil
		case "list":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":[{"record":{"name":"vcpe-mesh","radioType":"mesh","lifecycle":"ready"},"resolvedKernel":{"radioId":7,"phyName":"vcpe-mesh"},"resolvedNamespace":{"pid":123,"inode":456},"live":true}]}`)}, nil
		case "doctor":
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"meshCapability":"unknown","checks":[]}}`)}, nil
		default:
			t.Fatalf("unexpected command %q", args[0])
			return CommandResult{}, nil
		}
	}))
	if err := PreflightFeatures(context.Background(), 1, true, func(context.Context) (*Client, error) { return client, nil }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "version,list,doctor" {
		t.Fatalf("commands = %v, want a non-allocating doctor", commands)
	}
}

func TestPreflightRFScenarioRequiresHealthyRegisteredMedium(t *testing.T) {
	for _, test := range []struct {
		name    string
		health  string
		wantErr string
	}{
		{"healthy", `{"apiVersion":"vcpe.dev/hwsim/v1","command":"medium-health","success":true,"data":{"healthy":true,"checks":[{"name":"baseline","ok":true},{"name":"service","ok":true},{"name":"socket","ok":true},{"name":"registration","ok":true}]}}`, ""},
		{"unregistered", `{"apiVersion":"vcpe.dev/hwsim/v1","command":"medium-health","success":true,"data":{"healthy":true,"checks":[{"name":"baseline","ok":true},{"name":"service","ok":true},{"name":"socket","ok":true},{"name":"registration","ok":false}]}}`, "registration"},
		{"old manager", `{"apiVersion":"vcpe.dev/hwsim/v1","command":"medium-health","success":false,"error":{"code":"unknown_command","message":"unsupported command"}}`, "medium-health"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var commands []string
			client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
				commands = append(commands, strings.Join(args, " "))
				switch args[0] {
				case "version":
					return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"mesh"}}`)}, nil
				case "list":
					return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":[]}`)}, nil
				case "doctor":
					return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"meshCapability":"supported","checks":[]}}`)}, nil
				case "medium-health":
					return CommandResult{Stdout: []byte(test.health), ExitCode: map[bool]int{true: int(ExitNotReady), false: 0}[test.name == "old manager"]}, nil
				default:
					t.Fatalf("unexpected command %q", args[0])
					return CommandResult{}, nil
				}
			}))
			err := PreflightRequirements(context.Background(), 1, true, true, func(context.Context) (*Client, error) { return client, nil })
			if test.wantErr == "" && err != nil || test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("preflight error = %v, want %q", err, test.wantErr)
			}
			if strings.Join(commands, ",") != "version,list,doctor --probe,medium-health" {
				t.Fatalf("commands = %v", commands)
			}
		})
	}
}

func TestPreflightRejectsIncompatibleAPI(t *testing.T) {
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v2","command":"version","success":true,"data":{"version":"test"}}`)}, nil
	}))
	err := Preflight(context.Background(), 1, func(context.Context) (*Client, error) { return client, nil })
	if err == nil || !strings.Contains(err.Error(), "incompatible hwsim API") {
		t.Fatalf("error = %v", err)
	}
}

func TestPreflightRejectsUnhealthyDoctor(t *testing.T) {
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		if args[0] == "version" {
			return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"test"}}`)}, nil
		}
		return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":false,"checks":[]}}`)}, nil
	}))
	err := Preflight(context.Background(), 1, func(context.Context) (*Client, error) { return client, nil })
	if err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf("error = %v", err)
	}
}

package hwsim

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type transportFunc func(context.Context, ...string) (CommandResult, error)

func (fn transportFunc) Execute(ctx context.Context, args ...string) (CommandResult, error) {
	return fn(ctx, args...)
}

type pidInspectorFunc func(context.Context, string) (int, error)

func (fn pidInspectorFunc) ContainerPID(ctx context.Context, name string) (int, error) {
	return fn(ctx, name)
}

func TestClientCommandMappingAndDecoding(t *testing.T) {
	ensureJSON := `{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":true,"data":{"created":true,"reused":false,"radio":{"name":"vcpe-radio","permanentMac":"02:00:00:00:00:01","groupMask":2,"interfaceName":"wlan0","radioType":"station","namespace":{"pid":42,"inode":99},"kernel":{"radioId":1,"phyName":"vcpe-radio"},"lifecycle":"ready","updatedAt":"2026-09-30T00:00:00Z"}}}`
	var got []string
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		got = append([]string(nil), args...)
		return CommandResult{Stdout: []byte(ensureJSON)}, nil
	}))
	request := EnsureRequest{Name: "vcpe-radio", MAC: "02:00:00:00:00:01", GroupMask: 2, NetNSPID: 42, InterfaceName: "wlan0", RadioType: "station"}
	result, err := client.Ensure(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ensure", "--name", "vcpe-radio", "--mac", "02:00:00:00:00:01", "--group-mask", "2", "--netns-pid", "42", "--ifname", "wlan0", "--type", "station"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	if !result.Created || result.Reused {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientMapsRemainingManagerCommands(t *testing.T) {
	type exchange struct {
		args     []string
		response string
	}
	exchanges := []exchange{
		{args: []string{"version"}, response: `{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"6.1"}}`},
		{args: []string{"doctor", "--probe"}, response: `{"apiVersion":"vcpe.dev/hwsim/v1","command":"doctor","success":true,"data":{"healthy":true,"checks":[]}}`},
		{args: []string{"list"}, response: `{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":[]}`},
		{args: []string{"release", "--name", "vcpe-radio"}, response: `{"apiVersion":"vcpe.dev/hwsim/v1","command":"release","success":true,"data":{"name":"vcpe-radio","released":true,"alreadyAbsent":false}}`},
		{args: []string{"gc", "--keep", "vcpe-a", "--keep", "vcpe-z"}, response: `{"apiVersion":"vcpe.dev/hwsim/v1","command":"gc","success":true,"data":{"released":[],"preserved":["vcpe-a","vcpe-z"]}}`},
	}
	next := 0
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		if next >= len(exchanges) {
			t.Fatalf("unexpected command %#v", args)
		}
		exchange := exchanges[next]
		next++
		if !reflect.DeepEqual(args, exchange.args) {
			t.Fatalf("args = %#v, want %#v", args, exchange.args)
		}
		return CommandResult{Stdout: []byte(exchange.response)}, nil
	}))
	if _, err := client.Version(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Doctor(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Release(context.Background(), "vcpe-radio"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GC(context.Background(), []string{"vcpe-z", "vcpe-a"}, false); err != nil {
		t.Fatal(err)
	}
	if next != len(exchanges) {
		t.Fatalf("executed %d/%d commands", next, len(exchanges))
	}
}

func TestClientReturnsTypedCommandError(t *testing.T) {
	client := NewClient(transportFunc(func(context.Context, ...string) (CommandResult, error) {
		return CommandResult{
			Stdout:   []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":false,"error":{"code":"namespace_not_found","message":"namespace missing"}}`),
			ExitCode: int(ExitNotReady),
		}, nil
	}))
	_, err := client.Ensure(context.Background(), EnsureRequest{})
	var commandErr *CommandError
	if !errors.As(err, &commandErr) || commandErr.ExitCode != ExitNotReady || commandErr.Code != "namespace_not_found" {
		t.Fatalf("error = %#v", err)
	}
	if !IsErrorCode(err, "namespace_not_found") {
		t.Fatalf("IsErrorCode(%v) = false", err)
	}
}

func TestClientRejectsProtocolMismatch(t *testing.T) {
	tests := []struct {
		name   string
		result CommandResult
	}{
		{name: "api", result: CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v2","command":"version","success":true,"data":{"version":"dev"}}`)}},
		{name: "command", result: CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"list","success":true,"data":{"version":"dev"}}`)}},
		{name: "exit", result: CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"version","success":true,"data":{"version":"dev"}}`), ExitCode: 1}},
		{name: "multiple", result: CommandResult{Stdout: []byte("{}\n{}\n")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient(transportFunc(func(context.Context, ...string) (CommandResult, error) { return test.result, nil }))
			if _, err := client.Version(context.Background()); err == nil {
				t.Fatal("Version() error = nil")
			}
		})
	}
}

func TestClientReportsMissingManagerOutput(t *testing.T) {
	client := NewClient(transportFunc(func(context.Context, ...string) (CommandResult, error) {
		return CommandResult{Stderr: []byte("sudo: /usr/libexec/vcpe/vcpe-hwsim: command not found"), ExitCode: 127}, nil
	}))
	_, err := client.Version(context.Background())
	if err == nil || !strings.Contains(err.Error(), "command not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientValidatesReturnedIdentity(t *testing.T) {
	client := NewClient(transportFunc(func(context.Context, ...string) (CommandResult, error) {
		return CommandResult{Stdout: []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":true,"data":{"created":true,"reused":false,"radio":{"name":"vcpe-other","permanentMac":"02:00:00:00:00:01","groupMask":2,"interfaceName":"wlan0","radioType":"station","namespace":{"pid":42,"inode":99},"kernel":{"radioId":1,"phyName":"vcpe-other"},"lifecycle":"ready","updatedAt":"2026-09-30T00:00:00Z"}}}`)}, nil
	}))
	_, err := client.Ensure(context.Background(), EnsureRequest{Name: "vcpe-radio", MAC: "02:00:00:00:00:01", GroupMask: 2, NetNSPID: 42, InterfaceName: "wlan0", RadioType: "station"})
	if err == nil {
		t.Fatal("Ensure() error = nil")
	}
}

func TestEnsureForContainerRetriesOneFreshPIDOnMissingNamespace(t *testing.T) {
	pids := []int{41, 42}
	inspectCalls := 0
	inspector := pidInspectorFunc(func(_ context.Context, name string) (int, error) {
		if name != "vcpe-edge-gateway-1" {
			t.Fatalf("container name = %q", name)
		}
		pid := pids[inspectCalls]
		inspectCalls++
		return pid, nil
	})
	ensureCalls := 0
	client := NewClient(transportFunc(func(_ context.Context, args ...string) (CommandResult, error) {
		ensureCalls++
		pid := args[8]
		if ensureCalls == 1 {
			return CommandResult{
				Stdout:   []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":false,"error":{"code":"namespace_not_found","message":"namespace missing"}}`),
				ExitCode: int(ExitNotReady),
			}, nil
		}
		pidNumber, _ := strconv.Atoi(pid)
		response := fmt.Sprintf(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":true,"data":{"created":true,"reused":false,"radio":{"name":"vcpe-radio","permanentMac":"02:00:00:00:00:01","groupMask":2,"interfaceName":"wlan0","radioType":"station","namespace":{"pid":%d,"inode":99},"kernel":{"radioId":1,"phyName":"vcpe-radio"},"lifecycle":"ready","updatedAt":"2026-09-30T00:00:00Z"}}}`, pidNumber)
		return CommandResult{Stdout: []byte(response)}, nil
	}))
	request := EnsureRequest{Name: "vcpe-radio", MAC: "02:00:00:00:00:01", GroupMask: 2, InterfaceName: "wlan0", RadioType: "station"}
	result, err := client.EnsureForContainer(context.Background(), inspector, "vcpe-edge-gateway-1", request)
	if err != nil {
		t.Fatal(err)
	}
	if inspectCalls != 2 || ensureCalls != 2 || result.Radio.Namespace.PID != 42 {
		t.Fatalf("inspect calls = %d, ensure calls = %d, result = %#v", inspectCalls, ensureCalls, result)
	}
}

func TestEnsureForContainerDoesNotRetryOtherErrors(t *testing.T) {
	inspectCalls := 0
	inspector := pidInspectorFunc(func(context.Context, string) (int, error) {
		inspectCalls++
		return 41, nil
	})
	client := NewClient(transportFunc(func(context.Context, ...string) (CommandResult, error) {
		return CommandResult{
			Stdout:   []byte(`{"apiVersion":"vcpe.dev/hwsim/v1","command":"ensure","success":false,"error":{"code":"conflict","message":"radio conflict"}}`),
			ExitCode: int(ExitConflict),
		}, nil
	}))
	_, err := client.EnsureForContainer(context.Background(), inspector, "vcpe-edge-gateway-1", EnsureRequest{})
	if err == nil || inspectCalls != 1 {
		t.Fatalf("error = %v, inspect calls = %d", err, inspectCalls)
	}
}

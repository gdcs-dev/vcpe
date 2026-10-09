package runtimeinit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeviceWaiterContinuesWhenAllDevicesAppear(t *testing.T) {
	calls := 0
	waiter := DeviceWaiter{
		PollInterval: time.Millisecond,
		Exists: func(string) bool {
			calls++
			return calls > 1
		},
	}
	if err := waiter.Wait(context.Background(), []string{"wlan0"}, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("existence checks = %d, want at least 2", calls)
	}
}

func TestDeviceWaiterReportsSortedMissingDevices(t *testing.T) {
	waiter := DeviceWaiter{PollInterval: time.Millisecond, Exists: func(string) bool { return false }}
	err := waiter.Wait(context.Background(), []string{"wlan1", "wlan0", "wlan1"}, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "missing: wlan0, wlan1") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeviceWaiterReturnsImmediatelyWithoutDevices(t *testing.T) {
	called := false
	waiter := DeviceWaiter{Exists: func(string) bool {
		called = true
		return false
	}}
	if err := waiter.Wait(context.Background(), nil, 0); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("existence check called without devices")
	}
}

package runtimeinit

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

type DeviceWaiter struct {
	Exists       func(string) bool
	PollInterval time.Duration
}

func (w DeviceWaiter) Wait(ctx context.Context, devices []string, timeout time.Duration) error {
	names := uniqueDeviceNames(devices)
	if len(names) == 0 {
		return nil
	}
	if timeout <= 0 {
		return fmt.Errorf("radio device readiness timeout must be positive")
	}
	exists := w.Exists
	if exists == nil {
		exists = interfaceExists
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	missing := func() []string {
		result := make([]string, 0, len(names))
		for _, name := range names {
			if !exists(name) {
				result = append(result, name)
			}
		}
		return result
	}
	if absent := missing(); len(absent) == 0 {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for radio devices %s: %w", strings.Join(missing(), ", "), ctx.Err())
		case <-timer.C:
			return fmt.Errorf("radio device readiness timed out after %s; missing: %s", timeout, strings.Join(missing(), ", "))
		case <-ticker.C:
			if absent := missing(); len(absent) == 0 {
				return nil
			}
		}
	}
}

func uniqueDeviceNames(devices []string) []string {
	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		name := strings.TrimSpace(device)
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func interfaceExists(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}

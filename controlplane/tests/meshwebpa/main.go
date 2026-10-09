package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: meshwebpa <manifest>")
		os.Exit(2)
	}
	if err := runLive(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "mesh WebPA verification:", err)
		os.Exit(1)
	}
	fmt.Println("both mesh Gateways completed device-directed WebPA management")
}

func runLive(ctx context.Context, manifestPath string) error {
	document, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("load mesh manifest: %w", err)
	}
	if err := manifest.Validate(document); err != nil {
		return fmt.Errorf("validate mesh manifest: %w", err)
	}
	resolved, err := planner.Build(document, nil)
	if err != nil {
		return fmt.Errorf("plan mesh deployment: %w", err)
	}
	deviceIDs := make([]string, 0, 2)
	for _, name := range []string{"root", "remote"} {
		found := false
		for _, service := range resolved.Services {
			if service.Name != name || len(service.Instances) != 1 {
				continue
			}
			for _, iface := range service.Instances[0].Interfaces {
				if iface.Role == "wan" && iface.Device == "erouter0" && iface.MAC != "" {
					deviceIDs = append(deviceIDs, "mac:"+strings.ReplaceAll(strings.ToLower(iface.MAC), ":", ""))
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("%s has no planned WAN device identity", name)
		}
	}
	if deviceIDs[0] == deviceIDs[1] {
		return fmt.Errorf("root and remote share a WAN device identity")
	}
	container := document.Metadata.Name + "-webpa-1"
	readConfig := func(path string) ([]byte, error) {
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		contents, err := exec.CommandContext(readCtx, "podman", "exec", container, "cat", path).Output()
		if err != nil || len(contents) > maxResponseBytes {
			return nil, fmt.Errorf("read active %s configuration", path)
		}
		return contents, nil
	}
	talariaConfig, err := readConfig("/etc/talaria/talaria.yaml")
	if err != nil {
		return err
	}
	var talaria struct {
		Inbound struct {
			AuthKey string `yaml:"authKey"`
		} `yaml:"inbound"`
	}
	if err := yaml.Unmarshal(talariaConfig, &talaria); err != nil || talaria.Inbound.AuthKey == "" {
		return fmt.Errorf("invalid active Talaria authentication configuration")
	}
	scytaleConfig, err := readConfig("/etc/scytale/scytale.yaml")
	if err != nil {
		return err
	}
	var scytale struct {
		AuthToken string `yaml:"authToken"`
	}
	if err := yaml.Unmarshal(scytaleConfig, &scytale); err != nil || scytale.AuthToken == "" {
		return fmt.Errorf("invalid active Scytale authentication configuration")
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return verifyDevices(verifyCtx, containerClient(container), "http://127.0.0.1:6200", "http://127.0.0.1:6300", talaria.Inbound.AuthKey, scytale.AuthToken, deviceIDs)
}

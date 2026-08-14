package image

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runtimeInitServices lists services with a generated runtime-init and/or
// vcpe-healthd binary under services/<name>/container/platforms/<os>-<arch>/.
var runtimeInitServices = map[string]bool{
	"bng": true, "event-sink": true, "gateway": true, "oktopus": true,
	"routerd": true, "webpa": true, "xb10": true, "client": true,
}

// ServiceNameFromContext derives the service name from a build context path
// of the form "services/<name>".
func ServiceNameFromContext(buildContext string) string {
	return filepath.Base(filepath.Clean(buildContext))
}

// ParsePlatform splits an "os/arch" platform string, e.g. "linux/arm64".
func ParsePlatform(platform string) (goos, goarch string, err error) {
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid platform %q, expected os/arch", platform)
	}
	return parts[0], parts[1], nil
}

// PrepareBuild stages all generated binaries required by a build before the
// backend snapshots its context. Platform-qualified paths let a multi-platform
// build select the correct binary without mutating its context while it runs.
func PrepareBuild(ctx context.Context, req BuildRequest) error {
	return StageRuntimeInitBinaries(ctx, req.Context, req.Platforms)
}

// StageRuntimeInitBinaries rebuilds runtime-init and vcpe-healthd for every
// requested platform. With no explicit platforms, the script uses Linux and
// auto-detects the target architecture from the local container runtime.
func StageRuntimeInitBinaries(ctx context.Context, buildContext string, platforms []string) error {
	service := ServiceNameFromContext(buildContext)
	if !runtimeInitServices[service] {
		return nil
	}
	if len(platforms) == 0 {
		platforms = []string{""}
	}
	seen := make(map[string]bool, len(platforms))
	for _, platform := range platforms {
		if seen[platform] {
			continue
		}
		seen[platform] = true
		if err := stageRuntimeInitBinaries(ctx, service, platform); err != nil {
			return err
		}
	}
	return nil
}

func stageRuntimeInitBinaries(ctx context.Context, service, platform string) error {
	cmd := exec.CommandContext(ctx, "scripts/stage-runtime-init-binaries", service)
	cmd.Env = os.Environ()
	label := "native"
	if platform != "" {
		goos, goarch, err := ParsePlatform(platform)
		if err != nil {
			return err
		}
		cmd.Env = append(cmd.Env, "TARGET_GOOS="+goos, "TARGET_GOARCH="+goarch)
		label = platform
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stage runtime-init binaries for %s (%s): %w", service, label, err)
	}
	return nil
}

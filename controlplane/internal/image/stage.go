package image

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runtimeInitServices lists services with a committed runtime-init and/or
// vcpe-healthd binary under services/<name>/container/. Their Containerfiles
// COPY a host-built binary, so it must be (re)staged for the exact arch about
// to be built or a foreign-arch build silently bakes in the wrong-arch binary.
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

// StageRuntimeInitBinaries rebuilds and restages the committed runtime-init /
// vcpe-healthd binaries for the given build context and platform via
// scripts/stage-runtime-init-binaries, so a build always uses a binary
// matching the platform it's about to build for. It is a no-op for services
// with no committed runtime-init binaries. Assumes CWD is the repo root,
// matching the existing convention for BuildRequest.Context/File (relative
// paths).
//
// platform may be "" to mean "no explicit platform" (the request had no
// Platforms set): the script is invoked without TARGET_GOOS/TARGET_GOARCH so
// it falls back to its own defaults (GOOS always "linux" — container images
// are always Linux regardless of host OS — and GOARCH auto-detected from the
// Podman machine). Callers must NOT substitute the host OS/arch (e.g. Go's
// runtime.GOOS), since containers never run "darwin".
func StageRuntimeInitBinaries(ctx context.Context, buildContext, platform string) error {
	service := ServiceNameFromContext(buildContext)
	if !runtimeInitServices[service] {
		return nil
	}
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

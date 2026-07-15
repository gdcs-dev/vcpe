//go:build !homebrew

package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func init() {
	// Register developer-only commands in the central maps so they appear in
	// help output and are accepted as valid top-level commands.
	topLevelCommands["build"] = struct{}{}
	topLevelCommands["push"] = struct{}{}
	topLevelCommands["release"] = struct{}{}

	commandHelp["build"] = CommandHelp{
		Synopsis:    "Build or pull service images from a manifest",
		Description: "Resolves image actions (build, pull, or noop) for all services in the manifest without starting any containers. Respects the image pull policy declared in the manifest. Defaults to building a multi-arch OCI manifest list for linux/amd64 and linux/arm64; requires QEMU emulation on the Podman machine for cross-arch targets.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime for image operations (default: podman). With --backend docker, multi-arch builds use `docker buildx build --push` and push to the registry during build."},
			{Name: "--platform", Arg: "<csv>", Description: "Comma-separated OS/arch targets (default: linux/amd64,linux/arm64)"},
			{Name: "--no-cache", Description: "Disable layer cache when building images"},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: "Emit structured JSON output"},
		},
		Examples: []string{
			"vcpe build --manifest manifests/example.yaml",
			"vcpe build --manifest manifests/example.yaml --backend docker",
			"vcpe build --manifest manifests/example.yaml --platform linux/amd64",
		},
	}
	commandHelp["push"] = CommandHelp{
		Synopsis:    "Push service images from a manifest to their registries",
		Description: "Pushes all service images referenced in the manifest to their registries. The registry is derived from each service's image repository. Run `podman login <registry>` before pushing to authenticated registries.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime for push operations (default: podman)."},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
		},
		Examples: []string{
			"vcpe push --manifest manifests/example.yaml",
			"vcpe push --manifest manifests/example.yaml --backend docker",
		},
	}
	commandHelp["release"] = CommandHelp{
		Synopsis:    "Stamp manifest, commit, tag, push git, then build and push images",
		Description: "Requires --version <vX.Y.Z>. Sequence: (1) validate on main branch and that the tag doesn't exist; (2) stamp first-party image tags in the manifest; (3) git add + commit + tag + push to origin; (4) build all first-party service images as multi-arch OCI manifest lists with both the versioned tag and :latest and push to registry. Always defaults to the Docker backend.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML (will be stamped in place)"},
			{Name: "--version", Arg: "<vX.Y.Z>", Description: "Release version tag to create (e.g. v0.2.0); must not already exist in git"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime backend (default: docker)"},
			{Name: "--platform", Arg: "<os/arch,...>", Description: "Target platforms (default: linux/amd64,linux/arm64)"},
		},
		Examples: []string{
			"vcpe release --manifest manifests/example.yaml --version v0.2.0",
		},
	}

	developerCommandOrder = []string{"build", "push", "release"}
}

// dispatchDeveloperCommand routes build/push/release to their implementations.
func dispatchDeveloperCommand(opts Options) (daemon.CommandResponse, error) {
	switch opts.Command {
	case "build":
		return runBuild(opts)
	case "push":
		return runPush(opts)
	case "release":
		return runRelease(opts)
	default:
		return daemon.CommandResponse{}, fmt.Errorf("command %q is not executable", opts.Command)
	}
}

// runBuild resolves image actions for the manifest's services without applying
// runtime changes.
func runBuild(opts Options) (daemon.CommandResponse, error) {
	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}
	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64", "linux/arm64"}
	}
	mgr := image.New(newImageBackend(opts.Backend))
	summary, err := mgr.BuildWithOptions(context.Background(), doc, image.BuildOptions{NoCache: opts.NoCache, Platforms: platforms, ForceBuild: true})
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "build complete for deployment %q (platforms: %s)\n", doc.Metadata.Name, strings.Join(platforms, ","))
	for _, action := range summary.Actions {
		fmt.Fprintf(&b, "  %s (%s): %s\n", action.Service, action.Type, action.Action)
	}
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runPush pushes all service images from the manifest to their registries.
func runPush(opts Options) (daemon.CommandResponse, error) {
	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}
	backend := newImageBackend(opts.Backend)
	var b strings.Builder
	fmt.Fprintf(&b, "push complete for deployment %q\n", doc.Metadata.Name)
	for _, svc := range doc.Spec.Services {
		ref := image.ImageReference(svc.Image)
		if err := backend.PushImage(context.Background(), image.PushRequest{Reference: ref}); err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("push %s (%s): %w", svc.Name, ref, err)
		}
		fmt.Fprintf(&b, "  %s (%s): pushed\n", svc.Name, ref)
	}
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runRelease performs a full versioned release:
//  1. Stamp first-party image tags in the manifest (opts.Version).
//  2. git add → commit → tag → push (via runGitRelease).
//  3. Build all first-party service images as multi-arch with :version + :latest.
func runRelease(opts Options) (daemon.CommandResponse, error) {
	version := opts.Version // validated non-empty by CLI

	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}

	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64", "linux/arm64"}
	}

	backendName := opts.Backend
	if backendName == "" {
		backendName = "docker"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "release %s for deployment %q (platforms: %s)\n", version, doc.Metadata.Name, strings.Join(platforms, ","))

	if err := gitReleasePreflight(version); err != nil {
		return daemon.CommandResponse{}, err
	}

	if err := manifest.StampManifestFile(opts.ManifestPath, version); err != nil {
		return daemon.CommandResponse{}, fmt.Errorf("stamp manifest: %w", err)
	}
	fmt.Fprintf(&b, "manifest stamped: %s → tag: %s\n", opts.ManifestPath, version)

	if err := runGitRelease(opts.ManifestPath, version); err != nil {
		return daemon.CommandResponse{}, err
	}
	fmt.Fprintf(&b, "git: committed, tagged %s, and pushed to origin\n", version)

	backend := newImageBackend(backendName)
	for _, svc := range doc.Spec.Services {
		if svc.Image.BuildContext == "" {
			continue
		}
		versionedRef := fmt.Sprintf("%s:%s", svc.Image.Repository, version)
		latestRef := fmt.Sprintf("%s:latest", svc.Image.Repository)
		if err := backend.BuildImage(context.Background(), image.BuildRequest{
			Tags:      []string{versionedRef, latestRef},
			Context:   svc.Image.BuildContext,
			File:      svc.Image.Containerfile,
			Platforms: platforms,
		}); err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("release build %s (%s): %w", svc.Name, versionedRef, err)
		}
		fmt.Fprintf(&b, "  %s (%s): pushed as %s, %s\n", svc.Name, svc.Type, versionedRef, latestRef)
	}

	fmt.Fprintf(&b, "release %s complete", version)
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// gitReleasePreflight validates git state before any file or registry mutations.
func gitReleasePreflight(version string) error {
	branchOut, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("release: determine current branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch != "main" {
		return fmt.Errorf("release must be run from the main branch (current branch: %s)", branch)
	}

	tagOut, err := exec.Command("git", "tag", "-l", version).Output()
	if err != nil {
		return fmt.Errorf("release: check existing tags: %w", err)
	}
	if strings.TrimSpace(string(tagOut)) != "" {
		return fmt.Errorf("release: tag %s already exists; bump --version or delete the existing tag first", version)
	}
	return nil
}

// runGitRelease stages, commits, tags, and pushes the release.
func runGitRelease(manifestPath, version string) error {
	if out, err := exec.Command("git", "add", manifestPath).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git add %s: %w\n%s", manifestPath, err, strings.TrimSpace(string(out)))
	}
	msg := fmt.Sprintf("release: pin images to %s", version)
	if out, err := exec.Command("git", "commit", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git commit: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "tag", version).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git tag %s: %w\n%s", version, err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "push", "origin", "HEAD").CombinedOutput(); err != nil {
		return fmt.Errorf("release: git push origin HEAD: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "push", "origin", version).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git push origin %s: %w\n%s", version, err, strings.TrimSpace(string(out)))
	}
	return nil
}

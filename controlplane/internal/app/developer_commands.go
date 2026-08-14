//go:build !homebrew

package app

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func init() {
	// Register developer-only commands in the central maps so they appear in
	// help output and are accepted as valid top-level commands.
	topLevelCommands["build"] = struct{}{}
	topLevelCommands["push"] = struct{}{}
	topLevelCommands["stamp"] = struct{}{}
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
	commandHelp["stamp"] = CommandHelp{
		Synopsis:    "Pin image tags in one or more manifest files without touching git",
		Description: "Stamps every first-party service image (those with a buildContext) in the specified manifests to the target version. Does not perform any git operations. Run this before `vcpe release` to pin tags across multiple manifests and test them before the git tag is created.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path|glob>", Description: "Path or glob to manifest file(s); may be repeated"},
			{Name: "--version", Arg: "<vX.Y.Z>", Description: "Version tag to stamp into image.tag fields"},
		},
		Examples: []string{
			"vcpe stamp --version v0.3.0 --manifest manifests/example.yaml",
			"vcpe stamp --version v0.3.0 --manifest manifests/example.yaml --manifest manifests/example-macvlan.yaml",
			`vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml"`,
		},
	}
	commandHelp["release"] = CommandHelp{
		Synopsis:    "Commit, tag, push git, then build and push images for pre-stamped manifests",
		Description: "Requires --version <vX.Y.Z>. Sequence: (1) validate on main branch and that the tag doesn't exist; (2) collect manifest set from --manifest flags or auto-detect via git diff; (3) verify coherence (all first-party tags == version); (4) git add + commit + tag + push to origin; (5) build and push all first-party images (deduplicated) as multi-arch OCI manifests with :version and :latest. Run `vcpe stamp` first to pin tags across all manifests.",
		RequiredFlags: []FlagHelp{
			{Name: "--version", Arg: "<vX.Y.Z>", Description: "Release version tag to create (e.g. v0.3.0); must not already exist in git"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path|glob>", Description: "Path or glob to manifest file(s); may be repeated. If omitted, auto-detected via git diff"},
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime backend (default: docker)"},
			{Name: "--platform", Arg: "<os/arch,...>", Description: "Target platforms (default: linux/amd64,linux/arm64)"},
		},
		Examples: []string{
			"vcpe release --version v0.3.0",
			`vcpe release --version v0.3.0 --manifest "manifests/*.yaml"`,
		},
	}

	developerCommandOrder = []string{"build", "push", "stamp", "release"}
}

// dispatchDeveloperCommand routes build/push/stamp/release to their implementations.
func dispatchDeveloperCommand(opts Options) (daemon.CommandResponse, error) {
	switch opts.Command {
	case "build":
		return runBuild(opts)
	case "push":
		return runPush(opts)
	case "stamp":
		return runStamp(opts)
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

// runStamp pins image tags in one or more manifest files without touching git.
// Use this before `vcpe release` to stamp multiple manifests and test them
// before the git tag is created.
func runStamp(opts Options) (daemon.CommandResponse, error) {
	version := opts.Version // validated non-empty by CLI
	if version == "" {
		return daemon.CommandResponse{}, fmt.Errorf("stamp requires --version <vX.Y.Z>")
	}
	paths := opts.ManifestPaths

	var b strings.Builder
	stamped := 0
	for _, path := range paths {
		doc, err := manifest.Load(path)
		if err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("stamp %s: %w", path, err)
		}
		if err := Preflight(doc); err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("stamp %s: %w", path, err)
		}
		if err := manifest.StampManifestFile(path, version); err != nil {
			return daemon.CommandResponse{}, err
		}
		fmt.Fprintf(&b, "  stamped: %s → %s\n", path, version)
		stamped++
	}
	fmt.Fprintf(&b, "stamped %d manifest(s) to %s", stamped, version)
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runRelease performs a full versioned release:
//  1. Validate git state (main branch, tag absent).
//  2. Collect manifest set: ManifestPaths if provided, else auto-detect via git diff.
//  3. Verify coherence: every first-party service tag == version.
//  4. Build and push images (deduplicated across all manifests).
//  5. git add → commit → tag → push (via runGitRelease).
func runRelease(opts Options) (daemon.CommandResponse, error) {
	version := opts.Version // validated non-empty by CLI

	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64", "linux/arm64"}
	}
	backendName := opts.Backend
	if backendName == "" {
		backendName = "docker"
	}

	if err := gitReleasePreflight(version); err != nil {
		return daemon.CommandResponse{}, err
	}

	// Collect manifest set: explicit --manifest flags, or auto-detect via git diff.
	manifestPaths := opts.ManifestPaths
	if len(manifestPaths) == 0 {
		detected, err := detectStampedManifests()
		if err != nil {
			return daemon.CommandResponse{}, err
		}
		if len(detected) == 0 {
			return daemon.CommandResponse{}, fmt.Errorf(
				"no stamped manifests detected; run `vcpe stamp --version %s --manifest <path>` first,\n"+
					"or provide explicit --manifest flags", version)
		}
		manifestPaths = detected
	}

	// Coherence check: every first-party service in each manifest must be stamped to version.
	type buildKey struct{ repo, context, platforms string }
	seen := map[buildKey]bool{}
	type buildTarget struct {
		name, repo, ctx, containerfile string
		platforms                      []string
	}
	var builds []buildTarget
	var deploymentNames []string

	for _, path := range manifestPaths {
		doc, err := manifest.Load(path)
		if err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("release: load %s: %w", path, err)
		}
		deploymentNames = append(deploymentNames, doc.Metadata.Name)
		for _, svc := range doc.Spec.Services {
			if svc.Image.BuildContext == "" {
				continue
			}
			if svc.Image.Tag != version {
				return daemon.CommandResponse{}, fmt.Errorf(
					"coherence check failed: %s service %q has tag %q, expected %q;\n"+
						"run `vcpe stamp --version %s --manifest %s` first",
					path, svc.Name, svc.Image.Tag, version, version, path)
			}
			svcPlatforms := platforms
			if len(svc.Image.Platforms) > 0 {
				svcPlatforms = svc.Image.Platforms
			}
			sorted := append([]string(nil), svcPlatforms...)
			sort.Strings(sorted)
			k := buildKey{svc.Image.Repository, svc.Image.BuildContext, strings.Join(sorted, ",")}
			if !seen[k] {
				seen[k] = true
				builds = append(builds, buildTarget{
					name:          svc.Name,
					repo:          svc.Image.Repository,
					ctx:           svc.Image.BuildContext,
					containerfile: svc.Image.Containerfile,
					platforms:     svcPlatforms,
				})
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "release %s for deployments: %s (platforms: %s)\n",
		version, strings.Join(deploymentNames, ", "), strings.Join(platforms, ","))

	backend := newImageBackend(backendName)
	requests := make([]image.BuildRequest, len(builds))
	for i, t := range builds {
		versionedRef := fmt.Sprintf("%s:%s", t.repo, version)
		latestRef := fmt.Sprintf("%s:latest", t.repo)
		requests[i] = image.BuildRequest{
			Tags:      []string{versionedRef, latestRef},
			Context:   t.ctx,
			File:      t.containerfile,
			Platforms: t.platforms,
		}
	}

	if _, skip := backend.(noopImageBackend); !skip {
		for i := range requests {
			if err := image.PrepareBuild(context.Background(), requests[i]); err != nil {
				return daemon.CommandResponse{}, fmt.Errorf("release prepare %s: %w", builds[i].name, err)
			}
			requests[i].ArtifactsPrepared = true
		}
	}

	releaseCtx, cancelRelease := context.WithCancel(context.Background())
	defer cancelRelease()
	jobs := make(chan int, len(builds))
	errs := make(chan error, len(builds))
	for i := range builds {
		jobs <- i
	}
	close(jobs)

	workerCount := len(builds)
	if workerCount > 3 {
		workerCount = 3
	}
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for i := range jobs {
				t := builds[i]
				request := requests[i]
				versionedRef := request.Tags[0]
				if err := backend.BuildImage(releaseCtx, request); err != nil {
					errs <- fmt.Errorf("release build %s (%s): %w", t.name, versionedRef, err)
					cancelRelease()
					return
				}
				// Multi-platform builds push as part of manifest-list creation.
				if len(t.platforms) <= 1 {
					for _, ref := range request.Tags {
						if err := backend.PushImage(releaseCtx, image.PushRequest{Reference: ref}); err != nil {
							errs <- fmt.Errorf("release push %s (%s): %w", t.name, ref, err)
							cancelRelease()
							return
						}
					}
				}
			}
		}()
	}
	workers.Wait()
	close(errs)
	if err, ok := <-errs; ok {
		return daemon.CommandResponse{}, err
	}

	for i, t := range builds {
		versionedRef := requests[i].Tags[0]
		latestRef := requests[i].Tags[1]
		fmt.Fprintf(&b, "  %s: pushed as %s, %s\n", t.name, versionedRef, latestRef)
	}

	if err := runGitRelease(manifestPaths, version); err != nil {
		return daemon.CommandResponse{}, err
	}
	fmt.Fprintf(&b, "git: committed, tagged %s, and pushed to origin\n", version)

	fmt.Fprintf(&b, "release %s complete", version)
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// detectStampedManifests runs git diff to find modified YAML files under manifests/.
func detectStampedManifests() ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", "--", "manifests/").Output()
	if err != nil {
		return nil, fmt.Errorf("release: detect stamped manifests: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ".yaml") || strings.HasSuffix(line, ".yml") {
			paths = append(paths, line)
		}
	}
	return paths, nil
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

// runGitRelease stages all manifest files, commits, tags, and pushes the release.
func runGitRelease(manifestPaths []string, version string) error {
	args := append([]string{"add"}, manifestPaths...)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git add: %w\n%s", err, strings.TrimSpace(string(out)))
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

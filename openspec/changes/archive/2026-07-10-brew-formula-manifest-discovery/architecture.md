# Architecture: Brew Formula Manifest Discovery

## Overview

This change delivers two tightly coupled improvements:

1. **Formula overhaul** — Replace the stale `homebrew-vcpe/Formula/vcpe.rb` (which installs nine retired shell-script wrappers) with a Go-build formula that compiles the real `vcpe` binary from the `development` branch, installs it to `bin/`, and stages the repository's `manifests/*.yaml` to Homebrew's `pkgshare` directory.

2. **Manifest discovery in the binary** — Add a search-path resolver inside the vcpe control plane so every command that currently requires `--manifest <path>` can infer the manifest when the user omits the flag. Add `vcpe manifest list` as a new subcommand that surfaces all discovered manifests across the search path.

Together these changes make `vcpe apply` and `vcpe build` work out of the box after a `brew install vcpe` with no path argument required when a single deployment manifest is available.

## Components

### `homebrew-vcpe/Formula/vcpe.rb` (full rewrite)
The formula gains a `go` build dependency, compiles `controlplane/cmd/vcpe`, installs the resulting binary, and stages `manifests/` into pkgshare. All nine shell-script wrappers (`bng`, `gateway`, `webpa`, `routerd`, `xb10`, `client`, `net`, `homebrew-tap`) are removed. The formula tracks the `development` branch via a sha256-pinned tarball (`url`) with a parallel `head` option; `scripts/sync-homebrew-vcpe` recomputes the sha256 on each sync.

### `scripts/homebrew-tap` (extend `render_formula`)
Adds `development` as a third channel alongside `main` and `release`. Env vars: `VCPE_HOMEBREW_CHANNEL=development`, `VCPE_HOMEBREW_DEV_SHA256` (optional, skip live download). All three channels now emit the Go-build formula shape — the shell-script formula body is retired for all channels.

### `controlplane/internal/manifest/discovery.go` (new)
Encapsulates the search-path resolver. Exports:
- `FindAll(dirs []string) ([]Entry, error)` — walks each directory for `*.yaml` files, parses header fields (`apiVersion`, `kind`, `metadata.name`, `metadata.annotations.description`), returns valid entries.
- `Resolve(name string, dirs []string) (path string, err error)` — searches dirs in order for `<name>.yaml`, returns first match.
- `SearchDirs(executableFn func() (string, error)) []string` — builds the search list from the injected executable-path function, `VCPE_MANIFEST_DIRS`, and standard locations. Accepting a function (not a string) makes the pkgshare path testable without invoking the real `os.Executable`.

### `controlplane/internal/app/commands.go` (extend)
- Add `"manifest"` to `topLevelCommands` map.
- Implement `runManifest(opts)` that dispatches on `opts.CommandArgs[0]` → `"list"` calls `runManifestList(opts)`.
- `runManifestList` calls `discovery.FindAll(dirs)` and renders a table or JSON.

### `controlplane/internal/app/cli.go` (extend `parseArgs`)
- After flag parsing, before `validateCommandShape`, call a new `resolveManifestPath(opts)` function when `opts.ManifestPath == ""` and the command is manifest-consuming (`build`, `up`/`apply`, `plan`).
- `resolveManifestPath` calls `discovery.FindAll` on `SearchDirs`; if exactly one result → populates `opts.ManifestPath`; if zero or multiple → returns structured error.
- `validateCommandShape` remains unchanged — it continues to require a non-empty `ManifestPath`. By the time it runs, the path is already resolved.

### `controlplane/internal/manifest/model.go` (extend)
- Add `Annotations map[string]string \`yaml:"annotations,omitempty"\`` to `Metadata` struct. Non-breaking; all existing manifests continue to parse.

## Key Architectural Decisions

### Formula tracks `development` branch as sha256-pinned tarball + `head` option
**Choice:** `url` points to `…/archive/refs/heads/development.tar.gz` with a sha256 computed and re-pinned by `scripts/sync-homebrew-vcpe` on each sync. A parallel `head` option allows `brew install --HEAD` for always-latest.
**Rationale:** Matches the existing `main`-channel pattern. Users get a stable installable snapshot; `brew install vcpe` works without `--HEAD`. The sha256 represents a point-in-time snapshot and is deliberately short-lived. `scripts/sync-homebrew-vcpe` already has the infrastructure to download and hash branch archives.
**Alternatives considered:** `head`-only formula — rejected because it forces all users to use `--HEAD` and prevents normal `brew install vcpe`. sha256-pinned with no `head` — rejected because developers following the branch need a way to always pull latest.

### Go binary, not shell scripts
**Choice:** Formula adds `go` as a build dependency and runs `cd controlplane && go build -o #{bin}/vcpe ./cmd/vcpe`. All shell-script wrapper installs are removed.
**Rationale:** The shell scripts (`bng`, `gateway`, etc.) are retired stubs that print an error and exit 2. Installing them is misleading and harmful to UX. The control plane is a Go binary; the formula must install it as such.
**Alternatives considered:** Keep wrappers for backward compatibility — rejected; they are intentionally broken and documented as such in AGENTS.md.

### Manifests installed to `pkgshare`
**Choice:** `pkgshare.install "manifests"` in the formula stages `manifests/*.yaml` to `$(brew --prefix)/share/vcpe/manifests/`.
**Rationale:** `pkgshare` is the canonical Homebrew location for formula-owned data files. It is readable by the installed binary and separated from user data.
**Alternatives considered:** `etc` (config files, not data), `share` with manual subdir — `pkgshare` is the precise method for this use case.

### Discovery uses raw `os.Executable()` — symlinks not resolved
**Choice:** The binary calls `os.Executable()` and does NOT call `filepath.EvalSymlinks` before computing the pkgshare path.
**Rationale:** On macOS, Homebrew installs `$(brew --prefix)/bin/vcpe` as a symlink. `os.Executable()` returns the symlink path. Walking `../share/vcpe/manifests/` from the symlink correctly yields `$(brew --prefix)/share/vcpe/manifests/` — exactly where `pkgshare` lands. Resolving the symlink first would navigate into the Cellar directory where no manifests are installed.
**Alternatives considered:** `filepath.EvalSymlinks` — rejected; produces wrong path in Homebrew context. Build-time prefix injection — rejected; breaks with non-standard Homebrew prefixes.

### Discovery order and `VCPE_MANIFEST_DIRS` override
**Choice:** Search order:
1. `--manifest <path-or-name>` (explicit, unchanged)
2. `$VCPE_MANIFEST_DIRS` (colon-separated; env override)
3. `<os.Executable()>/../share/vcpe/manifests/` (pkgshare)
4. `~/.vcpe/manifests/` (user-local)
5. `./manifests/` (CWD; dev/clone mode)

**Rationale:** Explicit always wins. Env var allows CI and custom setups. pkgshare covers the normal brew-installed user. `~/.vcpe/` allows personal manifests without touching the brew install. CWD enables `git clone && vcpe apply` workflows for contributors.
**Alternatives considered:** Omitting CWD — rejected; dev workflow would require a path argument every time.

### `--manifest` accepts path OR bare name; optional when unambiguous
**Choice:** Flag accepts: absolute path, relative path (contains `/` or ends in `.yaml`), or bare name (no `/`, no `.yaml` suffix). When omitted: exactly 1 discovered → auto-select silently (logged at DEBUG); 0 found → error; 2+ found → error listing names with hint to run `vcpe manifest list`.
**Rationale:** Bare-name selection (`--manifest single-gateway`) is more ergonomic than typing a full path. Auto-select on exactly one removes all friction for the common single-deployment homebrew user. Multi-manifest ambiguity is an explicit, helpful error — not silent wrong behavior.
**Alternatives considered:** Always require `--manifest` — rejected; defeats the goal. Prompt interactively — rejected; breaks non-interactive use (CI, pipes).

### Manifest discovery resolution happens in `executeLocal` pre-dispatch
**Choice:** A `resolveManifestPath(opts)` call is inserted in `parseArgs` (`cli.go`) after flag-parsing, before `validateCommandShape`. It mutates `opts.ManifestPath` in-place for manifest-consuming commands (`build`, `up`/`apply`, `plan`).
**Rationale:** Single resolution point. `validateCommandShape` can then keep checking `ManifestPath != ""` without modification. All downstream callers (`runBuild`, `runApply`, etc.) see a fully resolved path. The path also propagates correctly over the daemon socket in `executeViaDaemon` since resolution happens in `parseArgs` before the socket call.
**Alternatives considered:** Resolution in `executeLocal` — rejected; `validateCommandShape` already hard-errors on empty `ManifestPath` before `executeLocal` is reached, requiring changes to two files. Per-command resolution — rejected; duplicates logic across commands.

### `vcpe manifest list` as a nested subcommand
**Choice:** Add `"manifest"` to `topLevelCommands`. `runManifest` dispatches on `opts.CommandArgs[0]`. Only `list` is implemented initially.
**Rationale:** Nested grammar (`manifest list`) is composable — `manifest validate`, `manifest show <name>` are natural future additions. Flat `vcpe manifests` would be grammatically awkward and harder to extend.
**Alternatives considered:** `vcpe list --manifests` flag on existing command — rejected; `vcpe list` is semantically "list deployments", mixing concerns breaks the command's contract.

### CWD discovery as accepted security risk
**Choice:** `./manifests/` is the lowest-priority search path with no special validation beyond schema parsing.
**Rationale:** The blast radius of a CWD-injected manifest is "deploy attacker's manifest" in a local Podman environment. This is a development tool run by the repo owner; not a privileged daemon. The risk is explicitly accepted and documented.
**Alternatives considered:** Disable CWD search — rejected; breaks the `git clone && vcpe apply` contributor workflow.

## Data Flow

```
User runs: vcpe apply

parseArgs(opts)            ← cli.go
  │
  ├─ flag parsing
  │
  ├─ opts.ManifestPath == "" && command is build/apply/plan ?
  │     │
  │     └─ resolveManifestPath(opts)
  │           │  SearchDirs(os.Executable())
  │           │  → [VCPE_MANIFEST_DIRS, pkgshare, ~/.vcpe/manifests/, ./manifests/]
  │           │
  │           ├─ 0 found  → error "no manifests found; run vcpe manifest list"
  │           ├─ 1 found  → opts.ManifestPath = entry.Path  (DEBUG log)
  │           └─ 2+ found → error listing names + "specify --manifest <name>"
  │
  ├─ validateCommandShape(opts)   // ManifestPath now guaranteed non-empty
  │
  └─ dispatch → runApply(opts)
                  │
                  └─ manifest.Load(opts.ManifestPath)

User runs: vcpe manifest list

executeLocal(opts)
  │
  └─ dispatch → runManifest(opts)
                  │
                  └─ runManifestList(opts)
                       │
                       └─ discovery.FindAll(SearchDirs(os.Executable()))
                            │
                            ├─ scan dirs for *.yaml
                            ├─ parse apiVersion, kind, metadata.name, annotations.description
                            └─ render table (or --json)
```

## Integration Points

| Point | Direction | Notes |
|-------|-----------|-------|
| `pkgshare/manifests/` | formula → binary | formula installs manifests; binary discovers them |
| `os.Executable()` | binary runtime | resolves binary symlink to compute pkgshare path |
| `VCPE_MANIFEST_DIRS` | env → binary | colon-separated override; CI/custom setups |
| `~/.vcpe/manifests/` | user → binary | user-local manifests, not installed by formula |
| `executeViaDaemon` socket | client → daemon | `opts.ManifestPath` populated before socket call; daemon receives resolved path |
| `scripts/sync-homebrew-vcpe` | CI/dev → formula | recomputes sha256 from development branch, re-renders formula |
| `scripts/homebrew-tap render_formula` | tool → vcpe.rb | extended for `development` channel; all channels now emit Go-build shape |

## Security Model

- **No privilege escalation:** vcpe manages Podman containers in a user VM; it does not run as root.
- **CWD injection risk:** `./manifests/` search path means a malicious directory can inject a manifest if the user runs `vcpe` in it. Accepted risk for a local dev tool. Documented in `discovery.go`.
- **sha256 pinning:** The development branch tarball is sha256-pinned at sync time. Between syncs, an attacker who could alter the branch archive would need to produce the same sha256 (preimage resistance of SHA-256). Standard Homebrew risk model applies.
- **No manifest secrets:** Manifests contain no credentials. Secrets are resolved at apply time from separate sources, not stored in manifests.

## Error Handling Strategy

| Scenario | Behavior |
|----------|----------|
| `--manifest` omitted, 0 found | `error: no manifests found in search path; run \`vcpe manifest list\` to see search dirs` |
| `--manifest` omitted, 2+ found | `error: multiple manifests found: [name1, name2, ...]; specify --manifest <name>` |
| `--manifest <name>` given, not found in any dir | `error: no manifest named "<name>" found in search path` |
| `--manifest <path>` given, file missing | existing file-not-found error (unchanged) |
| `*.yaml` file found but not valid vcpe manifest | silently skipped in `FindAll`; not surfaced as error (discovery is best-effort) |
| `os.Executable()` fails | fall through to remaining search dirs; log WARN |

## Observability Strategy

- `discovery.go` logs at **DEBUG** level: which directories were searched, how many manifests found, which was auto-selected.
- `runManifestList` emits to stdout (not logs); `--json` flag for machine consumption.
- No new metrics; manifest discovery is fast (stat + partial YAML parse, no network I/O).

## Constraints

- **`os.Executable()` must NOT resolve symlinks** — correctness depends on the symlink remaining in the path calculation.
- **`metadata.annotations` is optional** — all existing manifests without this field must continue to parse without error.
- **`scripts/sync-homebrew-vcpe` must be run after each significant push to `development`** — the sha256 is a snapshot; formula goes stale if not resynced.
- **The `development` channel render in `scripts/homebrew-tap` must not alter the `main` or `release` channel output** — existing `main`-channel installs must continue to work during the transition.
- **Go version compatibility** — the formula builds with whatever `go` version Homebrew provides; `controlplane/go.mod` requires `go 1.25.0`. Formula must declare the minimum Go version constraint if Homebrew's default is older.
- **`vcpe down` does NOT use manifest discovery** — it operates from persisted state only; `resolveManifestPath` must not be called for `down`/`destroy`.

## Diagrams

### Formula install layout (post-change)

```
$(brew --prefix)/
    bin/
        vcpe  →  Cellar/vcpe/<ver>/bin/vcpe   (symlink)
    share/
        vcpe/
            manifests/
                example.yaml
                xb10-gateway.yaml
                single-gateway.yaml  (from repo manifests/)

~/.vcpe/
    manifests/
        my-custom.yaml   (user-managed, not installed by formula)
```

### Manifest search path resolution

```
os.Executable()  =  $(brew --prefix)/bin/vcpe   (symlink, NOT resolved)
                                  │
                   ../share/vcpe/manifests/
                                  │
                                  ▼
            Priority 1: $VCPE_MANIFEST_DIRS (colon-sep)
            Priority 2: $(brew --prefix)/share/vcpe/manifests/   ← pkgshare
            Priority 3: ~/.vcpe/manifests/
            Priority 4: ./manifests/   (CWD)
```

### `vcpe manifest list` output

```
$ vcpe manifest list
NAME              PATH                                         DESCRIPTION
single-gateway    /opt/homebrew/share/vcpe/manifests/...       Single gateway with BNG and WebPA
xb10-gateway      /opt/homebrew/share/vcpe/manifests/...       XB10 + gateway deployment
my-custom         /Users/alice/.vcpe/manifests/my-custom.yaml  (no description)

$ vcpe manifest list --json
[
  {"name":"single-gateway","path":"/opt/homebrew/share/vcpe/manifests/single-gateway.yaml","description":"Single gateway with BNG and WebPA"},
  ...
]
```

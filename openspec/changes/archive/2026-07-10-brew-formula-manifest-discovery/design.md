## Context

The vcpe control plane is a Go binary built from `controlplane/cmd/vcpe`. The Homebrew formula currently installs nine shell-script wrappers that are all retired stubs. The new binary's CLI uses a flat `topLevelCommands` map in `internal/app/cli.go`; commands are dispatched via `parseArgs` → `validateCommandShape` → `executeLocal`/`executeViaDaemon`. Manifests live in `manifests/` at the repo root and follow the `vcpe.dev/v1` schema.

Architecture and all implementation decisions are in `architecture.md` and `decisions.md`. This document covers the implementation-level details those don't address.

## Goals / Non-Goals

**Goals:**
- Replace the broken formula with a working Go-build formula targeting the `development` branch
- Add manifest discovery so `--manifest` is optional for ergonomic installs
- Add `vcpe manifest list` for discoverability
- Keep all changes backward-compatible: existing explicit `--manifest <path>` usage unchanged

**Non-Goals:**
- Replacing `podman-compose` with direct Podman API calls (future work)
- Supporting Windows or Linux Homebrew (linuxbrew) in this change
- Caching or indexing manifests (discovery is always a live scan)

## Decisions

### `discovery.go` package layout

```go
// Entry represents a discovered manifest.
type Entry struct {
    Name        string // metadata.name
    Path        string // absolute file path
    Description string // metadata.annotations.description, or ""
}

// FindAll scans dirs in order, returns all valid vcpe.dev/v1 Deployment manifests.
// Invalid files (wrong apiVersion/kind, unparseable YAML) are silently skipped.
func FindAll(dirs []string) ([]Entry, error)

// Resolve returns the path to the first <name>.yaml found in dirs.
// Returns ("", ErrNotFound) if no match.
func Resolve(name string, dirs []string) (string, error)

// SearchDirs returns the ordered search directory list.
// executableFn is injected (typically os.Executable) for testability.
func SearchDirs(executableFn func() (string, error)) []string
```

`FindAll` reads only the YAML header (stops after `metadata` block for performance). It only decodes `apiVersion`, `kind`, and `metadata.*` — it does NOT parse the full `spec`, keeping it fast for large manifests.

### `parseArgs` extension

`resolveManifestPath(opts *Options, command string)` is called inside `parseArgs` after flag-parsing, gated on `opts.ManifestPath == ""` and `isManifestCommand(command)`:

```go
func isManifestCommand(cmd string) bool {
    switch cmd {
    case "build", "up", "apply", "plan":
        return true
    }
    return false
}
```

`resolveManifestPath` calls `discovery.FindAll(discovery.SearchDirs(os.Executable))`. On exactly one result, it sets `opts.ManifestPath = entries[0].Path`. The function returns a structured error (not a `fmt.Errorf` string) so the caller can format the error message with the discovered names list.

### Formula build process

```ruby
def install
  cd "controlplane" do
    system "go", "build",
           "-ldflags", "-s -w",
           "-o", bin/"vcpe",
           "./cmd/vcpe"
  end
  pkgshare.install "manifests"
end
```

`-ldflags "-s -w"` strips debug symbols and DWARF data — reduces binary size by ~30%, standard for distribution builds.

### `scripts/homebrew-tap` development channel

New `render_formula` branch for `VCPE_HOMEBREW_CHANNEL=development`:
- URL: `https://github.com/gdcs-dev/vcpe/archive/refs/heads/development.tar.gz`
- `version "development"`
- sha256: computed via `curl` + `shasum -a 256` (same mechanism as `main` channel), overridable via `VCPE_HOMEBREW_DEV_SHA256`
- `head` option: `branch: "development"`

All three channels (`main`, `release`, `development`) now emit the same Go-build formula body. The shell-script install body is removed from `render_formula`.

### `vcpe manifest list` command dispatch

`runManifest(opts)` is added to `commands.go`. It receives the full `Options` and dispatches on `opts.CommandArgs[0]`:

```
"list" → runManifestList(opts)
```

Unknown subcommands return a help hint: `unknown manifest subcommand "<x>"; run \`vcpe manifest --help\``.

`runManifestList` output format (table, default):
```
NAME              PATH                                         DESCRIPTION
single-gateway    /opt/homebrew/share/vcpe/manifests/...       Single gateway with BNG and WebPA
```
Column widths are the max of header + content. No color — output must be pipeable.

`--json` output:
```json
[{"name":"single-gateway","path":"/opt/homebrew/share/vcpe/manifests/...","description":"Single gateway with BNG and WebPA"}]
```

Empty result (no manifests found): empty array `[]` for `--json`; message "no manifests found in search path" to stdout (exit 0) for table.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| `development` branch tarball sha256 goes stale between syncs | `scripts/sync-homebrew-vcpe` recomputes on each run; caveats note advises re-sync cadence |
| `os.Executable()` returns unexpected path in unusual installs | `VCPE_MANIFEST_DIRS` override lets users bypass discovery entirely |
| `go build` in Homebrew sandbox may fail on `go 1.25` toolchain requirement | `go.mod` `toolchain` directive controls download; Homebrew's Go will download matching toolchain automatically |
| Auto-select silently uses wrong manifest if user has exactly one unintended manifest in search path | DEBUG log line states which manifest was auto-selected; `vcpe manifest list` always shows what would be used |

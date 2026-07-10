## Why

The Homebrew formula for `vcpe` still installs nine retired shell-script wrappers that print an error on every invocation — it has never been updated for the rewritten Go control plane. Users who `brew install vcpe` get a broken tool. At the same time, the new vcpe binary requires an explicit `--manifest <path>` on every command, which creates friction for users who just want to run a named deployment scenario. These two gaps — broken install and required path argument — make the current brew experience non-functional.

## What Changes

- **BREAKING**: Remove all nine shell-script wrapper installs (`bng`, `gateway`, `webpa`, `routerd`, `xb10`, `client`, `net`, `homebrew-tap`) from the formula; install the Go binary instead.
- **BREAKING**: Formula adds `go` as a build dependency and builds the binary from the `development` branch source.
- **New**: Formula installs `manifests/*.yaml` to `$(brew --prefix)/share/vcpe/manifests/` (Homebrew pkgshare).
- **New**: `vcpe manifest list [--json]` subcommand — discovers and lists all available manifest files across the search path.
- **New**: `--manifest` flag becomes optional for `build`, `apply`/`up`, and `plan` — auto-selects when exactly one manifest is discovered; errors with a helpful hint when multiple are found.
- **New**: `internal/manifest/discovery.go` — manifest search-path resolver with a defined discovery order.
- **New**: `metadata.annotations` field added to the manifest `Metadata` struct (non-breaking, optional).
- Remove `python@3` formula dependency (now implicit via `podman-compose`).
- Update `scripts/homebrew-tap` to support the `development` channel and emit the Go-build formula shape for all channels.

## Capabilities

### New Capabilities
- `manifest-discovery`: A search-path resolver that finds manifest files from pkgshare, user-local (`~/.vcpe/manifests/`), `VCPE_MANIFEST_DIRS`, and CWD. Powers both `vcpe manifest list` and `--manifest` auto-selection.

### Modified Capabilities
- `local-control-plane-cli`: `--manifest` becomes optional for `build`, `apply`, and `plan`; bare-name resolution added.
- `desired-state-manifests`: `Metadata` struct gains optional `annotations` field.

## Impact

- **`homebrew-vcpe/Formula/vcpe.rb`**: Full rewrite — Go build, development branch, pkgshare manifests, updated test block and caveats.
- **`scripts/homebrew-tap`**: New `development` channel, all-channel formula shape update.
- **`controlplane/internal/manifest/discovery.go`**: New file.
- **`controlplane/internal/manifest/model.go`**: `Annotations` field added to `Metadata`.
- **`controlplane/internal/app/cli.go`**: `parseArgs` extended with `resolveManifestPath` before `validateCommandShape`.
- **`controlplane/internal/app/commands.go`**: `manifest` subcommand + `runManifestList`.
- **`controlplane/go.mod`**: No new external dependencies (stdlib only for discovery).
- **Existing users** on `brew install vcpe`: their formula currently installs broken stubs; the update fixes this. No functional regression since the stubs were already non-functional.

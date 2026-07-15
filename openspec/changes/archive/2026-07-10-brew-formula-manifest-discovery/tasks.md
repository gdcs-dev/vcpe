## 1. Manifest model: add annotations field

- [x] 1.1 Add `Annotations map[string]string \`yaml:"annotations,omitempty"\`` to `Metadata` struct in `controlplane/internal/manifest/model.go`
- [x] 1.2 Update any validation or snapshot tests that assert the exact shape of `Metadata` to accept the new optional field

## 2. Manifest discovery package

- [x] 2.1 Create `controlplane/internal/manifest/discovery.go` with types `Entry{Name, Path, Description}` and functions `FindAll`, `Resolve`, `SearchDirs`
- [x] 2.2 Implement `SearchDirs(executableFn func() (string, error)) []string`: reads `VCPE_MANIFEST_DIRS` (colon-sep, tilde-expand, skip empty/missing), appends `<executableFn()>/../../share/vcpe/manifests/`, `~/.vcpe/manifests/`, `./manifests/`
- [x] 2.3 Implement `FindAll(dirs []string) ([]Entry, error)`: scans each dir for `*.yaml`, reads header-only (apiVersion, kind, metadata.name, metadata.annotations.description), skips invalid/unparseable files
- [x] 2.4 Implement `Resolve(name string, dirs []string) (string, error)`: returns path to first `<name>.yaml` in dirs
- [x] 2.5 Create `controlplane/internal/manifest/discovery_test.go`: unit tests using temp dirs — `FindAll` (empty, one valid, multiple, invalid files), `Resolve` (found, not found), `SearchDirs` (VCPE_MANIFEST_DIRS set/unset, executable fn error)

## 3. CLI: `--manifest` auto-resolution

- [x] 3.1 Add `resolveManifestPath(opts *Options, command string) error` in `controlplane/internal/app/cli.go`
- [x] 3.2 Implement `isManifestCommand(cmd string) bool` returning true for `build`, `up`, `apply`, `plan`
- [x] 3.3 Implement the discrimination algorithm in `resolveManifestPath`: `os.Stat` first; path-like+missing → file-not-found; bare name → `discovery.Resolve`; omitted → `discovery.FindAll` (0→error, 1→auto-select, 2+→error-with-list)
- [x] 3.4 Call `resolveManifestPath` in `parseArgs` after flag-parsing, before `validateCommandShape`, gated on `isManifestCommand`
- [x] 3.5 Add unit tests for `resolveManifestPath`: explicit path, bare name, auto-select, multi-manifest error, no-manifest error

## 4. CLI: `vcpe manifest list` subcommand

- [x] 4.1 Add `"manifest"` to `topLevelCommands` map in `controlplane/internal/app/cli.go`
- [x] 4.2 Implement `runManifest(opts Options) error` in `controlplane/internal/app/commands.go` — dispatches on `opts.CommandArgs[0]`; unknown subcommand → help hint
- [x] 4.3 Implement `runManifestList(opts Options) error` — calls `discovery.FindAll(discovery.SearchDirs(os.Executable))`, renders table (auto-width columns) or `--json` array; empty result: `[]` / "no manifests found"
- [x] 4.4 Add integration test for `runManifestList`: table output and `--json` output with a temp directory of manifests

## 5. Homebrew formula rewrite

- [x] 5.1 Rewrite `homebrew-vcpe/Formula/vcpe.rb`: `url` pointing to `development` branch tarball, `version "development"`, `sha256` placeholder, `head` option for `development` branch, `depends_on "go" => :build`, `depends_on "podman"`, `depends_on "podman-compose"` (remove `python@3`), Go build step, `pkgshare.install "manifests"`, updated `caveats`, `test do` using `vcpe --help`
- [x] 5.2 Run `scripts/sync-homebrew-vcpe` to compute and fill the sha256 for the current development branch HEAD
- [x] 5.3 Validate formula syntax: `ruby -c homebrew-vcpe/Formula/vcpe.rb`

## 6. Update `scripts/homebrew-tap`

- [x] 6.1 Add `development` channel case to `render_formula` in `scripts/homebrew-tap`: channel-specific URL (`development` branch), `version "development"`, `VCPE_HOMEBREW_DEV_SHA256` env override, `head` pointing to `development`
- [x] 6.2 Update `render_formula` Go-build install body for all channels (replace shell-script install with Go build + pkgshare manifests install)
- [x] 6.3 Remove the old shell-script wrapper install loop from `render_formula`
- [x] 6.4 Update the `test do` block in `render_formula` to use `vcpe --help`
- [x] 6.5 Update `scripts/homebrew-tap` help/usage text to include `development` channel in the channel list

## 7. Documentation and validation

- [x] 7.1 Update `packaging/homebrew/README.md` to describe the new install process and the `development` channel
- [x] 7.2 Run `go test ./internal/manifest/...` and `go test ./internal/app/...` to confirm all new and existing tests pass
- [x] 7.3 Smoke-test locally: `brew install --build-from-source <path-to-formula>`, run `vcpe manifest list`, confirm manifests from pkgshare appear

## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| Remove shell-script wrappers | `homebrew-vcpe/Formula/vcpe.rb` — nine wrapper installs removed | No |
| Formula requires `go` build dep | Users installing `vcpe` via brew will need Go available at build time | No |
| `scripts/homebrew-tap` formula shape | `render_formula` now emits Go-build body for all channels; old shell-script body retired | No |

---

## Decisions

### Decision: Commands triggering manifest discovery
Recommendation: `build`, `apply`/`up`, `plan` only
Decision: Proceed with recommended approach
Rationale: These three commands need a manifest file to operate. `down`/`destroy` operates from persisted deployment state; `status`, `logs`, `list`, `config`, `state` identify deployments by `--name` from the state store. No manifest file is needed for any of these.

Q: Which commands trigger manifest auto-discovery when `--manifest` is absent?
A: `build`, `apply`/`up`, `plan` — no others

---

### Decision: Go version constraint in formula
Recommendation: `depends_on "go" => :build` (unversioned)
Decision: Proceed with recommended approach
Rationale: Homebrew's `go` formula tracks the latest stable release. `controlplane/go.mod` requires `go 1.25.0`; the Go toolchain handles forward-compatible builds. If Homebrew's Go is too old, the error is clear and actionable. Pinning `go@X.Y` only works once Homebrew creates a versioned formula for older releases.

Q: Should the formula pin a specific Go version?
A: No — `depends_on "go" => :build`

---

### Decision: Formula `test do` block
Recommendation: `assert_match "vcpe", shell_output("#{bin}/vcpe --help")`
Decision: Proceed with recommended approach
Rationale: The Homebrew test sandbox doesn't have a running Podman machine. `vcpe manifest list` depends on `os.Executable()` path resolution working correctly in the sandbox — unreliable. `vcpe --help` tests that the binary linked correctly and the help system runs without any runtime dependencies.

Q: What should the formula `test do` block verify?
A: `assert_match "vcpe", shell_output("#{bin}/vcpe --help")`

---

### Decision: `--manifest` value discrimination algorithm
Recommendation: `os.Stat(value)` first, then format heuristic
Decision: Proceed with recommended approach
Rationale: Users who type a path that exists should always have it used as-is. The `os.Stat`-first approach handles all realistic inputs without surprising users.

Concrete algorithm:
1. `os.Stat(value)` succeeds → use value as file path (absolute or CWD-relative)
2. `strings.Contains(value, "/") || strings.HasSuffix(value, ".yaml")` → return "file not found: `<value>`" error (do NOT fall through to name search)
3. Otherwise → bare name; search discovery dirs for `<name>.yaml`; return "no manifest named `<name>` found" if not in any dir

Q: How should the `--manifest` flag discriminate between a path and a bare name?
A: `os.Stat` first; format heuristic fallback; path-like but missing → file-not-found, not name-search fallback

---

### Decision: `podman-compose` runtime dependency
Recommendation: Keep `depends_on "podman-compose"`; remove `depends_on "python@3"`
Decision: Proceed with recommended approach
Rationale: `controlplane/internal/compose/adapter.go` invokes `podman-compose` as a subprocess. Removing it from the formula would produce `command not found` on every `vcpe apply`. `python@3` can be removed because `podman-compose` declares its own Python dependency when installed via Homebrew.

Q: Should `podman-compose` stay as a runtime dependency?
A: Yes — keep `podman-compose`, remove only `python@3`

---

### Decision: `VCPE_MANIFEST_DIRS` handling
Recommendation: Silently skip; always tilde-expand; drop empty segments
Decision: Proceed with recommended approach
Rationale: Consistent with every Unix `$PATH`-style variable. Users may pre-configure paths (like `~/.vcpe/manifests`) before creating them; warning on every invocation would be noise. If no manifests are found anywhere, the "no manifests found" error surfaces the problem.

Rules:
- Split on `:`, drop empty segments (from `::` or trailing `:`)
- Expand `~` to `os.UserHomeDir()` before stat check
- Non-existent directories: silently skipped (not logged, not errored)

Q: How should `VCPE_MANIFEST_DIRS` handle missing dirs and malformed entries?
A: Silently skip; tilde-expand; drop empty segments

---

### Decision: `head` option branch
Recommendation: Both `url` and `head` track `development` branch
Decision: Proceed with recommended approach
Rationale: Consistency — `url` is the sha256-pinned snapshot of `development`; `head` is the always-latest form of the same branch. Using `main` for `head` while `url` tracks `development` would make `--HEAD` install a different (older) codebase.

Formula shape:
```ruby
url "https://github.com/gdcs-dev/vcpe/archive/refs/heads/development.tar.gz"
version "development"
sha256 "<computed by scripts/sync-homebrew-vcpe>"
head "https://github.com/gdcs-dev/vcpe.git", branch: "development"
```

Q: Which branch should the `head` option track?
A: `development` — same as `url`

---

### Decision: Unit testing strategy for `discovery.go`
Recommendation: Unit tests in `discovery_test.go` with `os.Executable` injected as a function parameter
Decision: Proceed with recommended approach
Rationale: Manifest discovery is pure file-system logic — fast, deterministic, trivially unit-testable. Injecting `executableFn func() (string, error)` as a parameter to `SearchDirs` (or the constructor) allows tests to point the pkgshare path at a temp directory without any mocking framework.

Test cases required:
- `FindAll`: empty dir, one valid manifest, multiple manifests, file with wrong `apiVersion`/`kind`, dir does not exist
- `Resolve`: name found, name not found, multiple dirs with same name (first wins)
- `SearchDirs`: `VCPE_MANIFEST_DIRS` set, unset, tilde expansion, `os.Executable` failure fallback
- `runManifestList`: table output and `--json` output via CLI handler integration test

Q: How should `discovery.go` be tested?
A: Unit tests with `executableFn func() (string, error)` injected; integration test for `runManifestList`

---

### Decision: `resolveManifestPath` insertion point
[BREAKING]
Recommendation: `parseArgs` in `cli.go`, after flag parsing, before `validateCommandShape`
Decision: Proceed with recommended approach
Rationale: `validateCommandShape` currently hard-errors on empty `ManifestPath` for manifest-consuming commands. Inserting resolution before this check means `validateCommandShape` can remain unchanged — it will see a non-empty resolved path. `executeViaDaemon` then forwards an already-resolved absolute path (clean). The alternative (relax `validateCommandShape`, resolve in `executeLocal`) requires modifying both `cli.go` and `execute.go` and would require server-side resolution for the daemon path.

Affected files: `controlplane/internal/app/cli.go` (`parseArgs` function only)

Q: Where should `resolveManifestPath` be called in the CLI dispatch chain?
A: In `parseArgs` (cli.go), after flag parsing, before `validateCommandShape`

---

### Decision: `SearchDirs` signature (architecture.md correction)
Recommendation: `SearchDirs(executableFn func() (string, error)) []string`
Decision: Correct the architecture.md which incorrectly shows `SearchDirs(executablePath string)`
Rationale: The function signature must accept a function (not a string) so unit tests can inject a fake `os.Executable` return value. The architecture.md was written with a string parameter; the correct form is the injected function, consistent with the testing decision.

Q: Should `SearchDirs` take a string path or an injected function?
A: Injected function `func() (string, error)` — architecture.md updated accordingly

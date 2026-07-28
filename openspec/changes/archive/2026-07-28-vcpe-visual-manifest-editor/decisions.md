## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| ServiceType interface additions | `controlplane/internal/typeregistry/registry.go`, all 7 type implementations under `controlplane/internal/types/` | No |
| ExpectedRoles enrichment for bng | `controlplane/internal/types/bng/bng.go`, preflight validation for any BNG service missing a `wan` interface | No |
| ExpectedRoles enrichment for gateway | `controlplane/internal/types/gateway/gateway.go`, preflight validation for any gateway service missing expected roles | No |

---

## Decisions

### Decision: vcpe binary discovery
Recommendation: VS Code configuration setting `vcpe.binaryPath` with PATH fallback; cache types in extension state after first successful fetch
Decision: Proceed with recommended approach
Rationale: Matches language extension conventions (Go, Rust, Python); PATH fallback handles common cases; caching avoids palette breakage if binary disappears mid-session; `vcpe.binaryPath` setting gives users an explicit override for non-standard install locations

Q: How does VcpeBinaryClient locate the `vcpe` binary?
A: Use recommended

---

### Decision: Invalid YAML error handling
Recommendation: Show error overlay on canvas with "Open in Text Editor" button; canvas frozen until YAML is valid; auto-recovers when fixed
Decision: Proceed with recommended approach
Rationale: Prevents misleading partial renders; the escape hatch keeps the user unblocked; auto-recovery on fix makes the feedback loop tight

Q: What does the canvas show when the manifest YAML is invalid?
A: Use recommended

---

### Decision: Secrets representation
Recommendation: Secrets, `maxReplicasPerService`, and `maxActiveDeployments` live in a "Deployment Settings" drawer in the property panel — no canvas nodes
Decision: Proceed with recommended approach
Rationale: Secrets are deployment-level references, not per-service topology; placing them on the canvas would misrepresent the data model; the settings drawer alongside spec-level scalar fields is the correct grouping

Q: How are `spec.secrets[]` and spec-level scalar fields represented?
A: Use recommended

---

### Decision: dependsOn toggle location
Recommendation: Canvas toolbar button (`⇢ Dependencies`), defaults on, not persisted across sessions
Decision: Proceed with recommended approach
Rationale: The toggle is a contextual visibility control for canvas busyness — not a persistent preference. A toolbar button is discoverable, immediate, and avoids polluting VS Code settings

Q: Where does the dependsOn toggle live?
A: Use recommended

---

### Decision: Service config editing
Recommendation: Raw YAML text editor widget in the ServiceForm property panel for v1; typed per-type forms deferred
Decision: Proceed with recommended approach
Rationale: Config schema currently lives only in the Go type system; exporting it requires significant additional scope; raw YAML editing is fully functional and unambiguous; typed forms are a natural v2 improvement

Q: How is the per-service `config` subtree edited?
A: Use recommended

---

### Decision: Layout sidecar git treatment
Recommendation: `.vcpe-layout.json` sidecar committed to git; layout is shared team context
Decision: Proceed with recommended approach
Rationale: A well-organized canvas diagram is shared value, not personal workspace data; the file is small, clean JSON, and produces minimal diff noise; consistent with how the team already commits the manifests themselves

Q: Should `.vcpe-layout.json` be committed to git?
A: Use recommended

---

### Decision: Extension publisher and name
Recommendation: `publisher: gdcs-dev`, `name: vcpe-visual-editor`, `displayName: vCPE Visual Manifest Editor`
Decision: Proceed with recommended approach
Rationale: Consistent with the existing GitHub org and container registry namespace (`ghcr.io/gdcs-dev/*`); leaves the door open for future marketplace publishing without a rename

Q: What is the VS Code extension publisher ID and name?
A: Use recommended

---

### Decision: Minimum VS Code version
Recommendation: `engines.vscode: "^1.85.0"` (November 2023)
Decision: Proceed with recommended approach
Rationale: `CustomTextEditorProvider` requires 1.46+; targeting 1.85 is a safe ~18-month-old baseline that all active developers are likely to meet; allows relying on stable webview and WorkspaceEdit APIs with confidence

Q: What minimum VS Code version should the extension target?
A: Use recommended

---

### Decision: Webview build tooling
Recommendation: Vite for webview React bundle; esbuild for extension host; single `npm run build` step
Decision: Proceed with recommended approach
Rationale: Vite is the current standard for React projects — fast HMR, simple config, first-class TypeScript support; esbuild is the standard VS Code extension host bundler; together they give the fastest dev iteration loop

Q: What build tooling should the webview use?
A: Use recommended

---

### Decision: Testing strategy
Recommendation: Vitest unit tests for `parse.ts`/`serialize.ts` (the YAML round-trip) only; no VS Code integration tests in v1
Decision: Proceed with recommended approach
Rationale: The YAML round-trip is the highest-risk code and is fully testable as pure TypeScript; VS Code integration tests require full Electron setup and are slow/fragile for a v1 internal tool; test coverage where it matters most

Q: What is the testing strategy for the extension?
A: Use recommended

---

### Decision: Network edge color scheme
Recommendation: Auto-assign a stable color per network role from a fixed palette (hash role name → color index); `NetworkBusNode` lane and all its `InterfaceEdge` lines share the same color
Decision: Proceed with recommended approach
Rationale: Makes topology immediately readable — users can trace which services share a network at a glance without reading labels; no user configuration needed; deterministic so the same role always gets the same color

Q: Should interface edge colors be derived from the network role?
A: Use recommended

---

### Decision: Extension distribution
Recommendation: Local `.vsix` package; `make build-extension` / `make install-extension` targets in root Makefile; no marketplace publishing in v1
Decision: Proceed with recommended approach
Rationale: The extension is internal tooling tightly coupled to the `vcpe.dev/v1` schema and `vcpe` binary; marketplace publishing adds publisher account overhead with no benefit for internal use

Q: How is the extension installed and distributed to the team?
A: Use recommended

---

### Decision: vcpe service types command
Recommendation: New top-level `service` command with `types` subcommand: `vcpe service types [--json]`
Decision: Proceed with recommended approach
Rationale: Consistent with the existing `manifest` + subcommand grouping pattern; extensible for future service-scoped subcommands without a rename; `--json` reuses the existing `opts.OutputJSON` flag pattern

Q: What should the new Go CLI command be called?
A: Use recommended

---

### Decision: Empty canvas welcome state
Recommendation: Welcome screen with manifest list and "New Manifest" button when editor opens without a file
Decision: Proceed with recommended approach
Rationale: A blank canvas with no context is not actionable; the welcome screen surfaces the manifest picker and creation action immediately; this state is rare in normal operation (editor usually opens with a specific file)

Q: What does the canvas show when opened without a file selected?
A: Use recommended

---

### Decision: Webview CSP and external resources
Recommendation: Fully self-contained — all assets bundled by Vite; no external CDN or runtime network requests; strict VS Code-recommended CSP with nonces
Decision: Proceed with recommended approach
Rationale: Works offline and in air-gapped environments; no attack surface from remote resources; VS Code recommended CSP posture; React Flow, icons (Lucide React), and fonts are npm dependencies bundled at build time

Q: Does the webview need external resources?
A: Use recommended

---

### Decision: Extension observability
Recommendation: Named VS Code Output Channel "vCPE Visual Editor"; info-level logs always on; no log-level config in v1
Decision: Proceed with recommended approach
Rationale: Silent failures in VS Code extensions (especially binary spawning and YAML sync) are very hard to diagnose; an output channel is the standard extension pattern and the only practical debug path without attaching a debugger

Q: Should the extension write logs to a VS Code Output Channel?
A: Use recommended

---

### Decision: New manifest skeleton
Recommendation: Minimal valid skeleton — `apiVersion`, `kind`, `metadata.name`, empty `networks: []`, `services: []`; created in `manifests/<name>.yaml`
Decision: Proceed with recommended approach
Rationale: Consistent with the blank-canvas decision (Q3 of explore session); no opinionated starter topology; user builds from scratch using the type palette

Q: What YAML skeleton is written when "New Manifest" is clicked?
A: Use recommended

---

### Decision: Initial auto-layout algorithm
Recommendation: `@dagrejs/dagre` for initial service node layout; network buses pinned; result persisted to sidecar immediately so dagre only runs once per manifest
Decision: Proceed with recommended approach
Rationale: Dagre produces clean layered layouts without the overlap problems of hand-rolled algorithms; MIT licensed, ~200KB, well-maintained; the one-shot persist approach means users pay the dagre cost only on first open

Q: What algorithm computes initial canvas positions when no sidecar exists?
A: Use recommended

---

### Decision: dependsOn arrow direction
Recommendation: Arrowhead points FROM dependent TO dependency (A → B = "A needs B")
Decision: Proceed with recommended approach
Rationale: Matches the `dependsOn` declaration semantics — the arrow originates at the entity making the requirement claim; consistent with Makefiles, Dockerfiles, and Kubernetes dependency graph conventions

Q: Which direction does the DependsOnEdge arrowhead point?
A: Use recommended

---

### Decision: Layout sidecar schema
Recommendation: `{ "version": 1, "nodes": { "<kind>:<identifier>": { "x": N, "y": N } } }`; node IDs are `network:<role>`, `service:<name>`, `nic:<parent>`; unknown IDs silently ignored; missing nodes get dagre positions
Decision: Proceed with recommended approach
Rationale: Named IDs are stable across YAML reordering (unlike array-index keys); `version` field enables safe future schema migration; ignoring unknown IDs handles deleted elements gracefully without errors; only storing x/y keeps the file minimal

Q: What is the schema for `.vcpe-layout.json`?
A: Use recommended

---

### Decision: ServiceType interface additions
[BREAKING]
Recommendation: Add `Description() string` and `DefaultImage() string` to the `ServiceType` interface; implement in all 7 registered types
Decision: Proceed with recommended approach
Rationale: The `vcpe service types --json` command needs human-readable descriptions and default image repositories to populate the visual editor's type palette; adding to the interface ensures all future type registrations provide this metadata

Q: (Derived from explore session and implementation coverage check)
A: Interface additions required; all 7 implementations updated as part of this change

---

### Decision: ExpectedRoles enrichment — bng
[BREAKING]
Recommendation: Change `bng.ExpectedRoles()` from `nil` to `[{wan, required}, {cm, optional}, {mgmt, optional}]`
Decision: Proceed with recommended approach
Rationale: The current `nil` return means the palette cannot suggest default interface wiring for BNG; enriching it makes the visual editor's drag-drop experience meaningful; the BREAKING risk is that BNG services without a `wan` interface will now fail preflight — acceptable since all real BNG deployments have a WAN interface

Q: (Derived from explore session; confirmed in implementation coverage check)
A: Enrich ExpectedRoles for bng; preflight impact is acceptable

---

### Decision: ExpectedRoles enrichment — gateway
[BREAKING]
Recommendation: Change `gateway.ExpectedRoles()` from `[{lan, optional}, {erouter, optional}]` to `[{wan, required}, {cm, optional}, {lan-p1, optional}]` (matching actual manifest usage)
Decision: Proceed with recommended approach
Rationale: Current roles (`lan`, `erouter`) are generic hints that don't match real manifest interface names (`wan`, `cm`, `lan-p1`..`lan-p4`); the palette should suggest the actual role names used in practice

Q: (Derived from explore session; confirmed in implementation coverage check)
A: Enrich ExpectedRoles for gateway; use actual manifest role names

---

### Decision: Network role rename in v1
Recommendation: Network role rename is blocked in v1 — role is read-only after creation; users delete and recreate if a role name change is needed
Decision: Proceed with recommended approach
Rationale: BNG's opaque `config` subtree embeds network role names (`access[].role`); safely renaming a role requires knowing the config schema for every service type, which is not available to the extension in v1; blocking rename eliminates a class of silent data corruption

Q: (Derived from explore session YAML write-back analysis)
A: Rename blocked v1; explicit scope constraint

---

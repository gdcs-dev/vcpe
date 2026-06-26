# AGENTS.md

vCPE: local Podman dev/test harness for containerized broadband components (BNG,
GATEWAY, WebPA, routerd, XB10, client). Operator entrypoint is the Go `vcpe` control
plane in `controlplane/`. It reconciles a desired-state manifest into Podman
projects (declarative: manifest -> plan -> apply).

## Current state: mid-rewrite, does NOT build

The branch is executing the `manifest-driven-redesign` change. `go build ./...`
and `go test ./...` currently FAIL. Do not treat this as a regression you caused;
verify against `git stash`/`HEAD` before assuming.

- The `manifest` schema was rewritten (task 1) but downstream consumers were not
  yet updated. Known broken: `internal/profile/*` (slated for deletion, task 9.1),
  `internal/app` (has only `*_test.go`, no impl yet — `app.ExecuteCLI`, referenced
  by both `cmd/vcpe/main.go` and the identical `cmd/vcpectl/main.go`, is
  undefined; this is why nothing links), `internal/manifest/validate_test.go`
  (uses removed `Version`/`Profile`/`Deployment` fields), and
  `internal/runtimeinit/servicecmd/run_test.go` (uses removed `CustomerID`).
- Progress and remaining work live in
  `openspec/changes/manifest-driven-redesign/tasks.md`. Sections 1–5 are largely
  done; 6–11 (renderers, `--customer`→`--name` CLI, validation, removals, tests,
  docs) are not. Read it before editing control-plane code.

## Stale docs vs. live code (do not copy blindly)

`README.md`, `docs/*`, and the root `manifest-bng-7.yaml` still show the OLD
schema (`version: v1alpha1`, `profile:`, `deployment.customer`, `--customer`).
The actual code in `controlplane/internal/manifest/model.go` is the NEW schema:
`apiVersion: vcpe.dev/v1`, `kind: Deployment`, `metadata.name` (deployment
identity; "customer" is at most a `metadata.labels` value), `spec.networks[]`,
`spec.services[]` (each with `type`, `image`, `interfaces[]`, opaque `config`).
Trust `model.go` and the change specs, not the README, until the docs tasks land.

## Build / test / commands

The Go module is nested at `controlplane/` (module
`github.com/gdcs-dev/vcpe/controlplane`), NOT the repo root. Run Go from there:

```bash
cd controlplane && go build -o bin/vcpe ./cmd/vcpe   # binary -> controlplane/bin/vcpe
cd controlplane && go test ./...
cd controlplane && go test ./internal/planner -run TestName   # single package/test
```

Root `Makefile` wrappers (`make build|up|status|down|smoke-go|release-gate`) just
shell out to `controlplane/bin/vcpe` and the smoke scripts; they own no logic.
`make release-gate` is the required pre-ship gate (`go test ./...` + all smoke
scripts under `tests/smoke/`). Podman-integration smokes
(`tests/smoke/controlplane-bng-*.sh`) need a working Podman runtime. Note the
Makefile is also pre-rewrite: `status`/`down`/`logs-*` still pass `--customer
$(CUSTOMER)` and `profile-list` calls `vcpe profile list` — all old-schema
surfaces being removed (tasks 8.1, 9.1). Only `build` and `release-gate` are safe
to trust verbatim today.

## Runtime-init binaries (container build flow)

`services/*/container/runtime-init-<svc>` are committed Linux/amd64 Go binaries
built from `controlplane/cmd/runtime-init-*`. Regenerate via
`scripts/stage-runtime-init-binaries [svc...]` (defaults linux/amd64; also stages
into `.local/artifacts/runtime-init/`). After changing `cmd/runtime-init-*` or the
`runtimeinit/contract`, restage these binaries — editing Go alone does not update
the images.

## Conventions / gotchas

- `scripts/{vcpe,bng,gateway,...}` are RETIRED stubs that only print an error and
  exit 2. Despite the README calling them "compatibility shims," they are not a
  working path. The only operator command is `vcpe`.
- This repo is spec-driven via the `openspec` CLI (installed). Specs are the
  source of truth: ratified specs in `openspec/specs/`, in-flight work in
  `openspec/changes/<name>/` (proposal/design/tasks/specs). Workflow skills/prompts
  live in `.github/skills/` and `.github/prompts/` (`opsx-*`). When changing
  control-plane behavior, reconcile with the relevant spec rather than diverging.
- `controlplane/`, `Makefile`, `openspec/`, and the new `tests/smoke/*` are
  UNTRACKED in git (the rewrite is not committed). Expect a noisy `git status`.
- macOS: `vcpe` auto-delegates Linux host-network commands to the Podman machine;
  force with `export VCPE_HOSTNET_DELEGATED=1`. Requires `podman machine` running.
- Persisted state is stamped `schemaVersion: vcpe.dev/v1`; the system refuses a
  mismatched/unstamped non-empty state root and (per design) requires an explicit
  `vcpe state reset` rather than auto-migrating.

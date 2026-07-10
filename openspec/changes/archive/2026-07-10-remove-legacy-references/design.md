## Context

The manifest-driven redesign (archived June 2026) and the legacy-bash-removal change completed all planned migrations: the control plane is fully Go-owned, `--name` replaced `--customer`, `profile` commands are gone, and service scripts delegate to `vcpe`. However, four legacy artefacts were not cleaned up in those changes:

1. `controlplane/cmd/vcpectl/` — a `main.go` stub identical to `cmd/vcpe/main.go`, never built separately in CI or Makefile.
2. `config/defaults.env` — sourced by old bash scripts; `VCPE_PROFILE`, `IMAGE_NAME`, and per-service image vars are not read by any live code path.
3. `Makefile` — `CUSTOMER ?= 7`, `--customer "$(CUSTOMER)"` in `status`/`down`/`logs-bng`, and `profile-list` target remain from before task 8.1 was applied to the Makefile.
4. `platform/scripts/lib/common.sh` — `VCPECTL_BIN`, `vcpectl_cmd()`, `run_vcpectl()`, `ensure_config_dirs()` creating a `profiles/` directory, and `require_customer_id()` remain from before the profile/customer model was removed.

Additionally two specs describe requirements that were correct during migration but are now obsolete: a `vcpectl` compatibility window and a "both run modes" developer workflow.

## Goals / Non-Goals

**Goals:**
- Delete `controlplane/cmd/vcpectl/` and `config/defaults.env` entirely.
- Fix the Makefile to use `--name` and remove the `profile-list` target.
- Remove the four dead helper symbols from `platform/scripts/lib/common.sh`.
- Retire the two stale spec requirements so the specs reflect current reality.

**Non-Goals:**
- Changing any runtime behaviour of the `vcpe` binary.
- Modifying service scripts under `services/*/scripts/` (they call `require_customer_id` defined inside their own scope, not via `common.sh`).
- Touching smoke tests or CI configuration.
- Removing `scripts/lib/` (it contains Python helpers used by `homebrew-tap`).

## Decisions

**Delete `cmd/vcpectl/` outright (not archive)**
The binary was always an alias. No Homebrew formula, Makefile target, or documented install path references `vcpectl` as a separate binary. Keeping it as dead code invites confusion about what the primary entry point is. No migration path needed — users who built it manually can rebuild `vcpe`.

**Delete `config/defaults.env` outright**
`grep -r 'defaults.env'` in the repo returns zero live callers. The file is not sourced by any current script; it predates the Go control plane. Deleting it removes a misleading source of old env var names (`VCPE_PROFILE`, `IMAGE_NAME`) that contradict manifest-driven image configuration.

**Makefile: replace `--customer` with `--name <name>` using a new `NAME` variable**
Introduce `NAME ?= bng-7` (matching the quick-start example deployment name) to replace `CUSTOMER ?= 7`. The `status`, `down`, and `logs-bng` targets become `--name "$(NAME)"`. This makes the Makefile consistent with current CLI contracts without breaking the convenience-wrapper intent.

**Remove `profile-list` target entirely (no replacement)**
`vcpe profile list` was removed as part of task 9.1. The target is broken today. No replacement is needed; operators use `vcpe status` to inspect active deployments.

**`platform/scripts/lib/common.sh`: surgical removal only**
Only the four dead symbols are removed. The rest of `common.sh` (bridge/NAT/firewall helpers, `run_linux_host*`, registry push) is still used by `services/*/scripts/`. Touching anything beyond the four dead symbols is out of scope.

## Risks / Trade-offs

**Risk: A caller of `require_customer_id` in `common.sh` is missed**
→ Mitigation: `grep -r 'require_customer_id' platform/` shows only `common.sh` defines it; callers are inside `services/bng/scripts/bng` and `services/client/scripts/client`, both of which define their own local copy (or call a local version). Verify before deleting.

**Risk: `vcpectl` binary still referenced in platform `common.sh`**
→ Mitigation: After removing `VCPECTL_BIN` / `vcpectl_cmd()` / `run_vcpectl()` from `common.sh`, `grep vcpectl platform/` should return zero hits. Verify at task time.

**Risk: Makefile `NAME` default conflicts with a real deployment**
→ `NAME ?= bng-7` is just a convenience default matching the quick-start; operators override it. No data loss risk.

## Open Questions

None — all decisions are clear from the current codebase state.

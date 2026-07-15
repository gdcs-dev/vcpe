## Why

The manifest-driven redesign and removal of legacy bash scripts are complete, but several legacy artefacts survived those changes: a duplicate `vcpectl` binary entrypoint, a `config/defaults.env` profile/image file the control plane no longer reads, stale `--customer` / `profile list` targets in the Makefile, and legacy helper functions (`vcpectl_cmd`, `run_vcpectl`, `require_customer_id`, `ensure_config_dirs` profiles path) in `platform/scripts/lib/common.sh`. These dead references cause confusion, keep the Makefile broken for day-to-day use, and leave two specs (`local-control-plane-cli`, `developer-readme-and-build-workflow`) describing compatibility windows and modes that no longer exist.

## What Changes

- **BREAKING** Delete `controlplane/cmd/vcpectl/` — the duplicate binary entrypoint identical to `cmd/vcpe`; `vcpe` is the sole operator command.
- Delete `config/defaults.env` — `VCPE_PROFILE`, `IMAGE_NAME`, `PODMAN_COMPOSE_BIN`, and the per-service image vars are not consumed by the control plane and have no callers.
- Fix `Makefile`: remove `CUSTOMER ?= 7` variable, replace `--customer "$(CUSTOMER)"` with `--name` on `status`, `down`, and `logs-bng`/`logs-webpa` targets, remove the `profile-list` target and its help text, and update all comments that reference the old flags.
- Clean `platform/scripts/lib/common.sh`: remove `VCPECTL_BIN`, `vcpectl_cmd()`, `run_vcpectl()`, the `$CONFIG_ROOT/profiles` dir creation from `ensure_config_dirs()`, and the `require_customer_id()` function.
- Update `local-control-plane-cli` spec: retire the `vcpectl` alias/debug-path requirement and the script-shim compatibility-window requirement — both are past their window.
- Update `developer-readme-and-build-workflow` spec: remove the "document both legacy script orchestration and control-plane compatibility mode" requirement and the "Makefile must wrap existing scripts" contract; replace with a simple convenience-wrapper requirement aligned to the current repo state.

## Capabilities

### New Capabilities
<!-- none — this is a removal/cleanup change -->

### Modified Capabilities
- `local-control-plane-cli`: Remove the `vcpectl` alias requirement and the script-shim compatibility-window requirement; `vcpe` is the sole operator command, no compatibility shims remain.
- `developer-readme-and-build-workflow`: Remove the "both run modes" and "Makefile wraps scripts" requirements; the Makefile is a pure convenience helper over `vcpe` commands.

## Impact

- **`controlplane/cmd/vcpectl/`** — deleted
- **`config/defaults.env`** — deleted
- **`Makefile`** — `CUSTOMER` var removed; `status`, `down`, `logs-bng` targets fixed to use `--name`; `profile-list` target and help text removed
- **`platform/scripts/lib/common.sh`** — `VCPECTL_BIN`, `vcpectl_cmd()`, `run_vcpectl()`, `ensure_config_dirs` profiles path, `require_customer_id()` removed
- **`openspec/specs/local-control-plane-cli/spec.md`** — two requirements retired
- **`openspec/specs/developer-readme-and-build-workflow/spec.md`** — two requirements updated
- No API changes; no state format changes; no build tag changes.

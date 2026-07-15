## 1. Delete Dead Entry Points And Config

- [x] 1.1 Delete `controlplane/cmd/vcpectl/` directory (contains only `main.go`, identical to `cmd/vcpe/main.go`)
- [x] 1.2 Delete `config/defaults.env` (VCPE_PROFILE, IMAGE_NAME, and per-service image vars — no live callers)
- [x] 1.3 Verify `grep -r 'vcpectl\|defaults\.env' controlplane/ scripts/ platform/ Makefile` returns zero hits after 1.1–1.2

## 2. Fix The Makefile

- [x] 2.1 Replace `CUSTOMER ?= 7` with `NAME ?= bng-7`
- [x] 2.2 Update `status` target: `$(VCPE_BIN) status --name "$(NAME)"`
- [x] 2.3 Update `down` target: `$(VCPE_BIN) down --name "$(NAME)" --force`
- [x] 2.4 Update `logs-bng` target: `$(VCPE_BIN) service bng logs --name "$(NAME)"`
- [x] 2.5 Remove `profile-list` target and its `.PHONY` entry
- [x] 2.6 Update `help` target echo lines to reference `NAME` (not `CUSTOMER`) and remove `profile-list` line
- [x] 2.7 Run `make help` and confirm output shows no `--customer` or `profile-list` references

## 3. Clean platform/scripts/lib/common.sh

- [x] 3.1 Remove `VCPECTL_BIN` variable declaration
- [x] 3.2 Remove `vcpectl_cmd()` function
- [x] 3.3 Remove `run_vcpectl()` function
- [x] 3.4 Remove `ensure_dir "$CONFIG_ROOT/profiles"` line from `ensure_config_dirs()`
- [x] 3.5 Remove `require_customer_id()` function
- [x] 3.6 Verify `grep -n 'vcpectl\|VCPECTL\|require_customer_id\|profiles' platform/scripts/lib/common.sh` returns zero hits

## 4. Update Specs

- [x] 4.1 Sync delta specs into main specs: run `openspec sync-specs --change remove-legacy-references` (or manually apply the REMOVED/MODIFIED blocks from each delta spec into `openspec/specs/local-control-plane-cli/spec.md` and `openspec/specs/developer-readme-and-build-workflow/spec.md`)
- [x] 4.2 Verify `openspec/specs/local-control-plane-cli/spec.md` contains no reference to `vcpectl` alias requirement or script-shim compatibility-window requirement
- [x] 4.3 Verify `openspec/specs/developer-readme-and-build-workflow/spec.md` contains no "both run modes" requirement and the Makefile requirement references `NAME` / `--name`

## 5. Verification

- [x] 5.1 Run `cd controlplane && go build ./...` — must succeed (removing `cmd/vcpectl` removes a package; verify no import cycle or missing dep)
- [x] 5.2 Run `cd controlplane && go test ./...` — must pass
- [x] 5.3 Run `make help` — output must not contain `--customer`, `CUSTOMER`, or `profile-list`
- [x] 5.4 Run `make status` — must invoke `vcpe status --name bng-7`
- [x] 5.5 Run `grep -r 'VCPE_PROFILE\|vcpectl\|--customer\|profile list\|profile-list\|require_customer_id' Makefile platform/scripts/lib/common.sh config/ 2>/dev/null` — must return empty

## 6. Delete Per-Service Bash Scripts

- [x] 6.1 Delete `services/bng/scripts/` (bng entrypoint + lib/render.sh, lib/podman.sh, lib/customer_config.py)
- [x] 6.2 Delete `services/client/scripts/client`
- [x] 6.3 Delete `services/gateway/scripts/gateway`
- [x] 6.4 Delete `services/routerd/scripts/routerd`
- [x] 6.5 Delete `services/webpa/scripts/` (webpa entrypoint + lib/podman.sh)
- [x] 6.6 Delete `services/xb10/scripts/xb10`

## 7. Delete Dead Service-Level Tests

- [x] 7.1 Delete `services/routerd/tests/` (routerd-7.sh, routerd-9.sh, routerd-20.sh — hardcoded old customer names, not wired into any test runner)

## 8. Delete Dead Platform Scripts

- [x] 8.1 Delete `platform/scripts/lib/bridges.sh` (no callers; Go hostnet owns bridge management)
- [x] 8.2 Delete `platform/scripts/lib/firewall.sh` (no callers; Go hostnet owns NAT/firewall)
- [x] 8.3 Delete `platform/scripts/net` (retirement stub; already prints error + exits 2)
- [x] 8.4 Delete `platform/tests/smoke/net-verify.sh` (orphaned; calls `vcpe status` without `--name`, not wired into release gate)

## 9. Delete Legacy Runtime Artifacts

- [x] 9.1 Delete `services/bng/runtime/` (old bash-rendered dhcpd/radvd/dnsmasq configs; Go renderer writes to state root, not here)
- [x] 9.2 Delete `services/webpa/runtime/` (empty placeholder — only contains .gitkeep)

## 10. Clean Remaining common.sh Dead Variables

- [x] 10.1 Remove `RUNTIME_ROOT`, `GATEWAY_RUNTIME_ROOT`, `ROUTERD_RUNTIME_ROOT` variable declarations from `platform/scripts/lib/common.sh` (only referenced by service scripts deleted in task 6)
- [x] 10.2 Remove `ensure_runtime_root()` function from `platform/scripts/lib/common.sh`
- [x] 10.3 Verify `grep -n 'RUNTIME_ROOT\|ensure_runtime_root' platform/scripts/lib/common.sh` returns zero hits

## 11. Final Verification

- [x] 11.1 Run `cd controlplane && go build ./...` and `go test ./...` — must pass
- [x] 11.2 Run `grep -r 'customer_id\|CUSTOMER_ID\|customer_config\.py\|bridges\.sh\|firewall\.sh\|bng/runtime\|webpa/runtime\|RUNTIME_ROOT' services/ platform/ 2>/dev/null | grep -v '.git\|bng/assets\|container\|compose\.yaml\|bng\.go\|gateway/src'` — must return empty

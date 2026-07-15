## 1. README.md — Installation Section

- [x] 1.1 Add a Homebrew installation option at the top of the Prerequisites/Build section: `brew tap gdcs-dev/vcpe && brew install vcpe`, followed by the existing `go build` path as the "from source" alternative
- [x] 1.2 Replace the verbose inline `manifest-bng-7.yaml` Quick Start block with: `vcpe manifest list` (discover available manifests), `vcpe manifest build` (create a new manifest interactively), and `vcpe up --manifest manifests/example.yaml` as the minimal first-run example

## 2. README.md — Commands Section

- [x] 2.1 Add a Commands reference table (or list) covering the full current surface: `init`, `build`, `push`, `release`, `up`/`apply`, `plan`, `down`/`destroy`, `list`, `manifest list`, `manifest build`, `status`, `logs`, `config`, `state`, `version` — each with a one-line description matching its `--help` synopsis
- [x] 2.2 Update the service types list to include all registered types: `bng`, `gateway`, `webpa`, `event-sink`, `xb10`, `oktopus`, `generic-container`
- [x] 2.3 Add a brief note on `ipamDriver: none` networks (skip Podman IPAM; container entrypoints assign IPs from explicit `interfaces[].ipv4` values)

## 3. README.md — Cleanup

- [x] 3.1 Remove or replace all remaining references to `manifest-bng-7.yaml`; use `--name <deployment>` in generic examples
- [x] 3.2 Update troubleshooting examples to use the generic `--name <deployment>` placeholder
- [x] 3.3 Verify the Makefile section still reflects actual available targets (`make build`, `make release-gate`); remove any stale targets

## 4. packaging/homebrew/README.md

- [x] 4.1 Fix the channels table: change the `release` channel notes from "Requires VCPE_HOMEBREW_VERSION..." to "Stable release (default)", and remove `development` as the default
- [x] 4.2 Replace `vcpe apply` with `vcpe up` in the usage examples (`vcpe up --manifest single-gateway`, `vcpe up`, etc.)
- [x] 4.3 Update the sync example to use `VCPE_HOMEBREW_CHANNEL=release` (the actual default) instead of `VCPE_HOMEBREW_CHANNEL=development`
- [x] 4.4 Add a brief mention of `vcpe manifest build` and `vcpe release` in the Usage section

## 5. services/event-sink/README.md

- [x] 5.1 Reframe "Starting the service": make `vcpe up --manifest manifests/example.yaml` (with event-sink declared in the manifest) the primary example; move the standalone `docker compose` invocation to a "Standalone / Development" sub-section
- [x] 5.2 Update the `docker compose` standalone example to use `podman compose` (consistent with the project's Podman focus) or note that either works

## 6. Spec Sync

- [x] 6.1 Apply MODIFIED `developer-readme-and-build-workflow` spec to `openspec/specs/developer-readme-and-build-workflow/spec.md`

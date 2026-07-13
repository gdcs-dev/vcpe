## Why

The control plane fabricates nearly all instance data (images, bridge names, podman project names, MAC addresses, gateways, per-service config) in Go from a single `deployment.customer` string, which directly violates its own ratified specs (`dynamic-topology-ipam`, `desired-state-manifests`) and makes onboarding a new deployment require source edits. This change makes the manifest the sole source of instance data so the system can be driven entirely by configuration.

## What Changes

- **BREAKING** Adopt a Kubernetes-style v1 manifest envelope: `apiVersion: vcpe.dev/v1`, `kind: Deployment`, `metadata{name,labels}`, `spec{networks[],services[]}`. `metadata.name` becomes the deployment identity; `customer` is demoted to an opaque label.
- **BREAKING** Replace the per-customer service catalog (`catalog/builtin.go`) with a behavior-only, compile-time **service-type registry**. Each service declares `services[].type` (the discriminator), and its `config` is decoded strictly and validated against a type-specific schema.
- **BREAKING** Promote `networks[]` to carry explicit `bridge` (defaulted), `nat`/`firewall` flags, and optional `ipv4`/`ipv6` blocks (each with `cidr`, optional `gateway`, optional `pool`), enabling v4-only/v6-only/dual-stack. IPAM becomes the sole IP authority; the deterministic-hash fallback assigns MACs only, keyed on `metadata.name/service/role[/index]`.
- **BREAKING** Add `services[].interfaces[]` that bind to networks by `role`, with optional `device`/`mac`/`ipv4`/`ipv6`. Explicit addresses/MACs are valid only when `replicas: 1`; `replicas > 1` uses IPAM and indexed MACs. Interface/bridge names are capped at 15 chars (IFNAMSIZ) with deterministic truncation.
- **BREAKING** Remove regex `template_replacements`; render the BNG `dhcpd.conf`/`dhcpd6.conf`/`radvd.conf` from a typed `config.access[]` model. Renderers are dispatched by service type; `generic-container` handles plain services; unsupported types fail at preflight.
- **BREAKING** Define a canonical `IFACE_<ROLE>_{NETWORK,DEVICE,MAC,IPV4,IPV6,GATEWAY4,GATEWAY6}` compose.env contract (plus `DEPLOYMENT_NAME`/`SERVICE_NAME`/`IMAGE`); curated compose files (bng/gateway/webpa) consume it, while `generic-container` compose is fully generated.
- **BREAKING** Replace the `--customer` selector with `--name` on `down`/`destroy`/`logs`/`status`/`service`.
- **BREAKING** Stamp persisted state with `schemaVersion: vcpe.dev/v1`; on mismatch the system refuses and requires an explicit `vcpe state reset` (no auto-migration).
- **BREAKING** Replace the `maxActiveCustomers` cap with `maxActiveDeployments` (distinct active `metadata.name`).
- Make `dependsOn` a control-plane cross-project lifecycle ordering contract (planner up-order / reverse teardown), distinct from intra-project compose `depends_on`.
- **BREAKING** Remove the legacy profile/`.env` compatibility layer, `config/profiles/*`, and `services/*/customers/*`.

## Capabilities

### New Capabilities
- `service-type-registry`: A behavior-only, compile-time registry that maps a service `type` to its validator, renderer, expected host-network roles, and default image policy; defines what "supported type" means and how `config` is strictly decoded and validated.

### Modified Capabilities
- `desired-state-manifests`: New v1 envelope (`apiVersion`/`kind`/`metadata`/`spec`), `type` discriminator, dual-stack `networks[]`, `interfaces[]`, replicas-vs-explicit-address rules, expanded validation rules, and the `maxActiveDeployments` cap.
- `dynamic-topology-ipam`: Explicit per-network names/bridges/gateways from the manifest, IPAM as the sole IP authority, MAC-only deterministic fallback on the canonical hash key, interface-role binding, and IFNAMSIZ-aware naming.
- `rendering-and-secrets-contract`: Renderer dispatch by service type, `generic-container` plus typed BNG renderer, removal of regex `template_replacements`, and the canonical `IFACE_<ROLE>_*` compose.env contract.
- `podman-reconciliation-engine`: Type-registry-driven service planning (replacing the per-customer typed catalog), schema-version state cutover, and cross-project `dependsOn` lifecycle ordering.
- `local-control-plane-cli`: `--name` deployment selection (replacing `--customer`) and the state schema-version reset guard.
- `developer-readme-and-build-workflow`: The README quick-start and run examples become manifest-driven (`vcpe.dev/v1` manifest + `--name`) instead of profile/`--customer` based.
- `profile-compat-translation`: **Removed** — the legacy profile import/translation capability is dropped for the greenfield manifest-driven model.

## Impact

- **Go control plane**: `manifest` (schema/validate), `catalog`→`typeregistry`, `render` (typed renderers, removes `bng_renderer.go` literals), `planner` (manifest-derived names + MAC-only hashing), `ipam` (sole authority), `runtimeinit/contract`, `app`/`daemon` (`--name`, `maxActiveDeployments`), `persist` (schema-version stamp + `state reset`), removal of `profile`.
- **Service assets**: curated `services/{bng,gateway,webpa}/compose.yaml` updated to the `IFACE_<ROLE>_*` env contract; `services/*/customers/*` and BNG `template_replacements` removed.
- **Config/state**: `config/profiles/*` removed; persisted state reset behind the schema-version stamp.
- **Docs**: `README.md`, `docs/architecture.md`, `docs/networking.md`, `docs/runbook.md`, and `packaging/homebrew/README.md` updated to the v1 manifest workflow and `--name` selection; profile/`--customer` references removed.
- **Tests**: golden render tests per type, schema validation table tests, registry-completeness test, planner determinism tests, smoke updated to v1 manifests; engine/journal/rollback tests preserved.

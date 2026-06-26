## BREAKING CHANGES

This is a greenfield, no-backward-compatibility cutover. The following decisions
require modifying or removing existing code.

| Decision | Affects | Override? |
|----------|---------|-----------|
| K8s-style manifest envelope (apiVersion/kind/metadata/spec) | `controlplane/internal/manifest/model.go`, `manifest/validate.go`, all manifests | No |
| `metadata.name` is identity; `customer` demoted to label | `controlplane/internal/planner/planner.go`, `app/app.go`, `runtimeinit/contract/contract.go` | No |
| Service discriminator `services[].type` (not `kind`) | `controlplane/internal/manifest/model.go`, `catalog` → `kindregistry` | No |
| Catalog → behavior-only type registry | `controlplane/internal/catalog/builtin.go` (replaced) | No |
| Typed renderers by type; remove regex `template_replacements` | `controlplane/internal/render/bng_renderer.go` (replaced), `services/bng/templates/*`, `services/bng/customers/*` (removed) | No |
| Canonical `IFACE_<ROLE>_*` compose.env contract | `controlplane/internal/render/*`, `services/{bng,gateway,webpa}/compose.yaml` | No |
| Replace `--customer` with `--name` on down/destroy/logs/status/service | `controlplane/internal/app/app.go`, `daemon/protocol.go`, command tests | No |
| State schema-version stamp + `vcpe state reset` | `controlplane/internal/persist/*`, `app/app.go` | No |
| Replace `maxActiveCustomers` with `maxActiveDeployments` | `controlplane/internal/manifest/model.go`, `manifest/validate.go`, `app` scaling limits | No |
| Delete profile/.env compat layer | `controlplane/internal/profile/*`, `config/profiles/*` | No |
| IPAM is sole IP authority; MAC-only deterministic fallback | `controlplane/internal/planner/planner.go`, `ipam/*`, `runtimeinit/contract/contract.go` | No |

---

## Decisions

### Decision: Manifest envelope and version
[BREAKING]
Recommendation: Kubernetes-style envelope — `apiVersion: vcpe.dev/v1`, `kind`, `metadata{name,labels}`, `spec{...}`
Decision: Proceed with recommended approach
Rationale: Familiar declarative shape; cleanly separates identity (`metadata`) from desired state (`spec`); drops `v1alpha1` for the clean greenfield cut.

Q: What top-level document shape and version identifier should the v1 manifest use?
A: K8s-style: apiVersion: vcpe.dev/v1, kind, metadata{name,labels}, spec{...}

---

### Decision: Top-level kind value and service discriminator name
[BREAKING]
Recommendation: Top-level `kind: Deployment`; per-service discriminator `services[].type`
Decision: Proceed with recommended approach
Rationale: Avoids the dual-`kind` collision; "type" reads naturally and yields clearer errors like `unsupported service type "foo"`.

Q: How do we name the top-level kind and the per-service discriminator to avoid the dual-'kind' collision?
A: Top-level kind: Deployment; service discriminator = services[].type

---

### Decision: Network schema (dual-stack, bridges, gateways, pools)
[BREAKING]
Recommendation: Per-network nested `ipv4{}`/`ipv6{}` blocks (each optional), optional `bridge` (default `<metadata.name>-<role>`), `nat`/`firewall` booleans, optional `pool{start,end}` ranges, optional `gateway` (default first usable host)
Decision: Proceed with recommended approach
Rationale: BNG/gateway need IPv6; optional families allow v4-only/v6-only/dual-stack; keeps IPAM as address authority with explicit-but-optional pools; gateways are network-level.

Q: What concrete schema should spec.networks[] use for dual-stack, bridges, gateways, and pools?
A: Nested ipv4{}/ipv6{} blocks (each optional), bridge optional w/ default, nat/firewall bools, optional pool ranges

Concrete shape:
```yaml
spec:
  networks:
    - role: wan
      bridge: edge-bng-wan          # optional; default <metadata.name>-<role>
      nat: true
      firewall: true
      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1, pool: { start: ..., end: ... } }
      ipv6: { cidr: 2001:dae:7:1::/64, gateway: ..., pool: { ... } }
```

---

### Decision: Service-level schema and image block
[BREAKING]
Recommendation: `{ name, type, replicas, image{repository,tag,buildContext?,containerfile?,pullPolicy}, dependsOn? }`; pull-policy vocabulary `build-if-missing|always-pull|never-build`
Decision: Proceed with recommended approach
Rationale: Reuses the already-implemented pull-policy vocabulary; `containerfile` makes build context + Containerfile path manifest-expressible.

Q: What concrete schema should spec.services[] use at the service/image level?
A: As recommended: name, type, replicas, image{repository,tag,buildContext,containerfile,pullPolicy}, dependsOn

---

### Decision: Interface schema
[BREAKING]
Recommendation: `{ role, device?, mac?, ipv4?, ipv6? }` — interface binds to a network by `role`; addresses are optional strings (one per family); MAC optional with deterministic fallback; gateway inherited from network
Decision: Proceed with recommended approach
Rationale: Keeps IPAM as the address authority, keeps v1 simple (single address per family), avoids restating network-level gateway.

Q: What concrete schema should services[].interfaces[] use?
A: As recommended: role, device?, mac?, ipv4? ipv6? (address strings, one per family)

---

### Decision: bng-type config schema; eliminate regex template_replacements
[BREAKING]
Recommendation: Typed `config.access[]` keyed by interface role with `radvd`/`dhcp4`/`dhcp6` objects; NAT CIDRs derived from `networks[].nat:true`; fully drop `template_replacements`
Decision: Proceed with recommended approach
Rationale: Regex substitution on `dhcpd.conf` is the exact anti-pattern the `rendering-and-secrets-contract` spec prohibits. Addressing already moves to `interfaces[]`; what remains BNG-specific (RA/DHCP) becomes typed fields. Deletes `services/bng/customers/*.yaml`.

Q: What concrete schema should the bng-type config use, and do we fully drop regex template_replacements?
A: Typed config.access[] keyed by role with radvd/dhcp4/dhcp6 objects; NAT derived from networks; drop template_replacements

Concrete shape:
```yaml
config:
  access:
    - role: wan
      radvd: { sendAdvert, managedFlag, otherConfigFlag, defaultLifetime, minDelayBetweenRAs, minRtrAdvInterval, maxRtrAdvInterval, prefixes? }
      dhcp4: { ranges: [{start,end}], options: { routers[], dnsServers[], domainName }, leaseSeconds }
      dhcp6: { ranges: [{start,end}], options: { dnsServers[] } }
```
`prefixes` defaults to the network ipv6 cidr.

---

### Decision: Remaining type config schemas and v1 type set
[BREAKING]
Recommendation: `gateway` config = `{ lan{ipv4,ipv6}, erouter{vlan?} }`; `webpa` config = empty (validate-empty); `client` folds into `generic-container`; `generic-container` config = `{ command?, env?, ports?, volumes? }`. v1 type set = `bng, gateway, webpa, generic-container`
Decision: Proceed with recommended approach
Rationale: gateway's only non-addressing config is the internal BRLAN0 LAN; webpa is asset-driven; client is just alpine. routerd/xb10 can be added later via the registry without schema changes.

Q: How do we define the remaining type config schemas and the v1 supported type set?
A: gateway{lan,erouter.vlan}, webpa(empty), client->generic-container, generic-container{command,env,ports,volumes}; type set = bng/gateway/webpa/generic-container

---

### Decision: Type registry and polymorphic config decoding
Recommendation: Compile-time registry with explicit `Register()`; `ServiceType` interface `{ Type(), ValidateConfig(yaml.Node) error, Renderer(), ExpectedRoles() []RoleRequirement, DefaultImagePolicy() }`; `config` captured as raw node and decoded via deferred STRICT decode (unknown fields error); "supported" iff registered
Decision: Proceed with recommended approach
Rationale: Explicit registration avoids scattered `init()` magic and gives one source of truth for "supported type"; strict decode catches config typos before mutation.

Q: How should the type registry and polymorphic config decoding work?
A: Compile-time registry, explicit Register(), ServiceType interface, deferred strict config decode (unknown fields error)

---

### Decision: Compose file sourcing (hybrid)
[BREAKING]
Recommendation: Registered curated types (`bng`/`gateway`/`webpa`) use `services/<type>/compose.yaml` (path from registry) + generated `compose.env`; `generic-container` has its compose file generated from manifest fields
Decision: Proceed with recommended approach
Rationale: Curated multi-process composes (e.g. webpa) stay hand-maintained; arbitrary services remain fully manifest-driven.

Q: How are compose files sourced — curated per type, fully generated, or hybrid?
A: Hybrid: known types use curated compose.yaml + generated env; generic-container compose fully generated

---

### Decision: CLI deployment-selection flag
[BREAKING]
Recommendation: Replace `--customer` with `--name` (`metadata.name`) on `down`/`destroy`/`logs`/`status`/`service`
Decision: Proceed with recommended approach
Rationale: Identity is now `metadata.name`; selecting by a demoted label would be confusing.

Q: How should deployment-selecting CLI commands identify a deployment now that metadata.name is the identity?
A: Replace --customer with --name (metadata.name) on down/destroy/logs/status/service [BREAKING]

---

### Decision: State schema-version cutover
[BREAKING]
Recommendation: `persist` stamps `schemaVersion: vcpe.dev/v1` on the state root; on missing/mismatched stamp with existing data, refuse with an actionable error directing the operator to run `vcpe state reset`; no auto-wipe
Decision: Proceed with recommended approach
Rationale: D2 renames nearly every resource, so diffing v1 against legacy state would mass-flag disruptive teardowns. Explicit, logged reset is safer than silent state destruction.

Q: What should happen when the persisted state root's schema-version stamp doesn't match v1?
A: Stamp schemaVersion; on mismatch refuse with error + require explicit 'vcpe state reset'

---

### Decision: Testing strategy
Recommendation: Golden-file render tests per type; schema validation table tests (valid + each failure mode); registry-completeness test; planner determinism tests; smoke updated to v1 manifests; keep engine/journal/rollback tests
Decision: Proceed with recommended approach
Rationale: Golden tests pin render output; table tests cover strict-decode and validation rules; completeness test guards the registry invariant.

Q: What testing strategy should this change adopt?
A: Golden render tests per type + validation table tests + registry-completeness + planner determinism + updated smoke

---

### Decision: Validation and edge-case rules
[BREAKING]
Recommendation: HARD ERROR — interface role with no matching network; duplicate service names; `dependsOn` cycle or unknown ref; explicit address outside network CIDR; unmet `expectedRoles`; `replicas` > `maxReplicasPerService`; unregistered type. WARNING — declared network referenced by no interface. ALLOWED — multiple interfaces sharing a role (indexed in hash key)
Decision: Proceed with recommended approach
Rationale: Fail fast on structural/topology errors before mutation; unused networks are benign; indexed duplicate roles support multi-attach services.

Q: What are the validation/preflight hard-fail vs warning rules?
A: As recommended (errors list + unused-network warning + indexed duplicate roles allowed)

---

### Decision: Replicas vs explicit addressing
[BREAKING]
Recommendation: Explicit `mac`/`ipv4`/`ipv6` valid only when `replicas: 1`; for `replicas > 1`, IPAM allocates per replica and MAC uses the deterministic hash with replica index
Decision: Proceed with recommended approach
Rationale: A single explicit address cannot fan out across N replicas; deferring to IPAM/hash per index is unambiguous.

Q: How are explicit addresses/MACs handled when replicas > 1?
A: Explicit mac/ip only if replicas:1; replicas>1 uses IPAM + indexed MAC

---

### Decision: compose.env variable contract
[BREAKING]
Recommendation: Canonical `IFACE_<ROLE>_{NETWORK,DEVICE,MAC,IPV4,IPV6,GATEWAY4,GATEWAY6}` scheme (role upper-cased, `-`→`_`, e.g. `lan-p1`→`LAN_P1`) plus `DEPLOYMENT_NAME`, `SERVICE_NAME`, `IMAGE`; curated `compose.yaml` files updated to consume it
Decision: Proceed with recommended approach
Rationale: Renderer needs a stable, documented env contract that curated compose files consume; per-role naming is deterministic and self-describing.

Q: What compose.env variable contract should the renderer emit?
A: Canonical IFACE_<ROLE>_* scheme; update curated compose files to consume it

---

### Decision: Interface/bridge name length (IFNAMSIZ)
Recommendation: Cap derived names at 15 chars; when `<metadata.name>-<role>` overflows, truncate and append a short deterministic hash suffix; warn on explicit names > 15
Decision: Proceed with recommended approach
Rationale: Linux IFNAMSIZ is 15; silent overflow fails bridge creation at runtime. Deterministic shortening keeps names stable.

Q: How to handle the 15-char interface/bridge name limit?
A: Cap 15 chars; truncate + deterministic hash suffix; warn on explicit >15

---

### Decision: Default route selection
[BREAKING]
Recommendation: Optional `interfaces[].defaultRoute: true` (≤1 per service); if unset, the `wan`-role interface wins; > 1 is a validation error
Decision: Proceed with recommended approach
Rationale: A container has one default route; an explicit marker with a sensible fallback removes ambiguity for multi-interface services.

Q: How is the default route chosen among multiple interfaces?
A: Optional interfaces[].defaultRoute (<=1); fallback to wan role; >1 = error

---

### Decision: NAT egress/uplink
Recommendation: Masquerade source CIDRs = networks with `nat: true`, egress via the host default uplink; not a manifest field in v1
Decision: Proceed with recommended approach
Rationale: Matches the existing single-masquerade firewall behavior; host uplink is an environment concern, not a per-deployment manifest concern.

Q: How is NAT egress/uplink determined?
A: Masquerade nat:true CIDRs via host default uplink; not a manifest field

---

### Decision: secretRef value source
Recommendation: `env` provider reads the host env var named by `key`; `file` provider reads the file at path `key`; resolved at apply time only, never persisted to state or logs
Decision: Proceed with recommended approach
Rationale: Matches existing `secrets.Resolve` and the `rendering-and-secrets-contract` requirement that secrets are never persisted.

Q: Where do secretRef values come from?
A: env reads host env var=key; file reads path=key; apply-time only, never persisted

---

### Decision: Active-deployment cap
[BREAKING]
Recommendation: Replace `maxActiveCustomers` with `maxActiveDeployments` counting distinct active `metadata.name`; drop the customer-coupled cap
Decision: Proceed with recommended approach
Rationale: `customer` is now an opaque label; the cap must count the real identity (`metadata.name`).

Q: What replaces the customer-coupled maxActiveCustomers cap?
A: Replace with maxActiveDeployments (distinct metadata.name); drop maxActiveCustomers

---

### Decision: Cross-type dependsOn ordering
Recommendation: `dependsOn` is a control-plane lifecycle ordering contract across separate compose projects (planner up-order, reverse teardown); it does NOT map to compose `depends_on`, which governs only intra-project container ordering inside a curated `compose.yaml`
Decision: Proceed with recommended approach
Rationale: Curated types are separate compose projects, so cross-service ordering must live in the planner; intra-project ordering stays in the curated compose file.

Q: How does cross-type dependsOn ordering work?
A: Control-plane lifecycle ordering across projects; not compose depends_on

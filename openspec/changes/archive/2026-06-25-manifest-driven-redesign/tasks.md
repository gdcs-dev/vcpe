## 1. Manifest schema (v1 envelope)

- [x] 1.1 Define v1 `Document{APIVersion,Kind,Metadata{Name,Labels},Spec}` structs in `internal/manifest/model.go`; remove `SupportedVersion="v1alpha1"` and `ProfileSpec`/`DeploymentSpec` customer coupling.
- [x] 1.2 Define `spec.networks[]` (`role`, optional `bridge`, `nat`, `firewall`, optional `ipv4`/`ipv6` blocks with `cidr`/`gateway`/`pool`) and `spec.services[]` (`name`, `type`, `replicas`, `image{repository,tag,buildContext,containerfile,pullPolicy}`, `dependsOn`, `interfaces[]`, raw `config`).
- [x] 1.3 Define `interfaces[]` (`role`, `device`, `mac`, `ipv4`, `ipv6`, `defaultRoute`); capture `service.config` as a deferred `yaml.Node`.
- [x] 1.4 Implement strict top-level decode (`KnownFields(true)`) and reject unsupported `apiVersion`/`kind` before planning.

## 2. Service type registry

- [x] 2.1 Create `internal/typeregistry` with `Register(ServiceType)`, `Lookup(type)`, `Registered()`, and the `ServiceType` interface (`Type`, `ValidateConfig(yaml.Node)`, `Renderer`, `ExpectedRoles`, `DefaultImagePolicy`) plus `RoleRequirement`.
- [x] 2.2 Delete `internal/catalog/builtin.go` and migrate any still-needed metadata into the registry contract.
- [x] 2.3 Add a `register.go` aggregator that wires the v1 type packages exactly once.

## 3. Service type packages

- [x] 3.1 `internal/types/bng`: typed `config.access[]` (`radvd`, `dhcp4`, `dhcp6`), validator, `ExpectedRoles`, `DefaultImagePolicy`; register.
- [x] 3.2 `internal/types/gateway`: typed `config{lan{ipv4,ipv6}, erouter{vlan}}`, validator, expected roles; register.
- [x] 3.3 `internal/types/webpa`: empty-config validator (validate-empty), expected roles; register.
- [x] 3.4 `internal/types/genericcontainer`: typed `config{command,env,ports,volumes}`, validator, expected roles; absorbs the former `client` service; register.
- [x] 3.5 Add a registry-completeness test asserting every v1 type supplies validator + renderer + expected roles.

## 4. IPAM as sole IP authority + identity helpers

- [x] 4.1 Make IPAM the only IP assigner: validate explicit interface addresses in-CIDR and reserve them; allocate all others from `networks[].pool`; reject overlaps.
- [x] 4.2 Extract `canonicalMAC(metaName, service, role, index)` (sha1 of `metadata.name/service/role[/index]`, first 5 bytes, `02:` prefix) as the sole MAC source.
- [x] 4.3 Extract `deriveBridgeName(metaName, role)` applying the `<name>-<role>` default and 15-char IFNAMSIZ truncation + deterministic hash suffix; warn on explicit names > 15.
- [x] 4.4 Add a cross-component determinism test asserting planner and `runtimeinit/contract` produce identical MACs/names for the same key.

## 5. Planner

- [x] 5.1 Rewire `internal/planner/planner.go` to derive network attachments from `networks[]` and interface identities from `interfaces[]`; remove `bng-%s`/`wan-%s`/`deterministicNetworkName(role, customer)` derivation.
- [x] 5.2 Replace the per-customer hash with the shared `canonicalMAC`/`deriveBridgeName` helpers; enforce explicit address/MAC only when `replicas == 1` and indexed identities when `replicas > 1`.
- [x] 5.3 Implement cross-project `dependsOn` lifecycle ordering (up-order / reverse teardown) distinct from intra-project compose `depends_on`.

## 6. Renderers + compose contract

- [x] 6.1 Move per-type renderers into the type packages behind a shared `Renderer` interface; delete `bng_renderer.go` embedded IP/MAC/gateway literals and `composeEnvForService`/`parseCustomer`.
- [x] 6.2 Implement the typed BNG renderer emitting `dhcpd.conf`/`dhcpd6.conf`/`radvd.conf` from typed fields; derive NAT CIDRs from `networks[].nat`. Remove `template_replacements` and `services/bng/customers/*`.
- [x] 6.3 Implement the `generic-container` renderer and fully generated compose document.
- [x] 6.4 Add the shared `ifaceEnv` producer emitting `IFACE_<ROLE>_{NETWORK,DEVICE,MAC,IPV4,IPV6,GATEWAY4,GATEWAY6}` + `DEPLOYMENT_NAME`/`SERVICE_NAME`/`IMAGE`.
- [x] 6.5 Update curated `services/{bng,gateway,webpa}/compose.yaml` to consume the canonical env contract.
- [x] 6.6 Make the render engine dispatch by `type`; unsupported type fails at preflight.

## 7. Validation + preflight

- [x] 7.1 Implement cross-object validation hard errors: unresolved interface role, duplicate service names, `dependsOn` cycle/unknown ref, explicit address outside CIDR, unmet expected roles, `replicas > max`, unregistered type, multiple `defaultRoute`.
- [x] 7.2 Emit the unused-network warning; allow multiple interfaces sharing a role (indexed).
- [x] 7.3 Dispatch per-type `config` validation via `typeregistry.Lookup` and add the strict-decode unknown-field error path.
- [x] 7.4 Make apply preflight reject unsupported types via `registry.Lookup` before mutation.

## 8. CLI, caps, and state cutover

- [x] 8.1 Replace `--customer` with `--name` on `down`/`destroy`/`logs`/`status`/`service` in `internal/app/app.go`, `daemon/protocol.go`, and forward through daemon/client.
- [x] 8.2 Replace `maxActiveCustomers` with `maxActiveDeployments` (distinct active `metadata.name`).
- [x] 8.3 Add the `schemaVersion: vcpe.dev/v1` stamp in `persist`; refuse non-empty mismatched/unstamped roots with an actionable error.
- [x] 8.4 Implement `vcpe state reset` to clear and re-stamp the state root.

## 9. Removals (greenfield cutover)

- [x] 9.1 Delete `internal/profile/*`, `config/profiles/*`, and remove the profile/config-import command surface.
- [x] 9.2 Delete `services/*/customers/*` and any remaining customer-ID fixtures.

## 10. Tests + smoke

- [x] 10.1 Add golden render tests per service type pinning exact `compose.env` and config artifacts.
- [x] 10.2 Add schema validation table tests (valid manifest + each failure mode from task 7.1).
- [x] 10.3 Add planner determinism tests; keep engine/journal/rollback tests passing.
- [x] 10.4 Convert `tests/smoke/*` and fixtures to v1 manifests (`apiVersion: vcpe.dev/v1`, `kind: Deployment`).
- [x] 10.5 Run `go build ./...` and `go test ./...` green; run smoke verification.

## 11. Documentation

- [x] 11.1 Rewrite the top-level `README.md` quick start to author a `vcpe.dev/v1` `Deployment` manifest, run `vcpe up`, and `vcpe status --name <metadata.name>`; remove `v1alpha1`, profile, `maxActiveCustomers`, and `--customer` references; update the project-layout/prerequisites bullets that mention `config/` profiles and customer flows.
- [x] 11.2 Update `docs/architecture.md` to describe the manifest-driven control plane, the service type registry, and IPAM as the sole IP authority; remove customer-ID/profile-based topology descriptions.
- [x] 11.3 Update `docs/networking.md` for manifest-declared `networks[]`/`interfaces[]`, dual-stack `ipv4`/`ipv6` blocks, deterministic MAC/bridge naming (IFNAMSIZ), and the `IFACE_<ROLE>_*` env contract.
- [x] 11.4 Update `docs/runbook.md` for `--name` deployment selection, `maxActiveDeployments`, and the `vcpe state reset` schema-version cutover procedure.
- [x] 11.5 Update `packaging/homebrew/README.md` and any example manifests to v1; verify no remaining references to profiles, customers, or `--customer` across docs.
- [x] 11.6 Verify all README/runbook command examples execute against a v1 manifest end-to-end (per the developer-readme spec quick-start scenario).

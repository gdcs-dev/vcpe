## 1. Bounded Callback Source Support

- [x] 1.1 Generalize the fixed AppArmor simulator socket eligibility in `controlplane/internal/diagnostic/cpe_callback.go` without accepting arbitrary socket paths, WRP payloads, or emitters.
- [x] 1.2 Parameterize `NewCPEWebPACallbackProvider` in `controlplane/internal/diagnostic/provider.go` by supported source type while retaining the shared `webpa` target, `event-sink` subscriber constraint, and ordered graph.
- [x] 1.3 Register the callback provider for both Gateway and XB10 in `controlplane/internal/types/types.go`; preserve rejection for every other CPE source type.

## 2. XB10 Health Capability

- [x] 2.1 Configure `services/xb10/container/vcpe-healthd.service` with the fixed root-only AppArmor simulator diagnostic socket and `cpe-webpa-callback` journey advertisement.
- [x] 2.2 Add health-daemon configuration coverage proving XB10 advertises the callback journey only through the existing diagnostic-handler path and preserves the passive CPE and Parodus journeys.

## 3. Focused Automated Coverage

- [x] 3.1 Extend callback probe tests to prove the fixed socket accepts only the existing validated bounded request and exact accepted response behavior for an explicitly configured XB10 source.
- [x] 3.2 Extend provider, registry, resolver, and orchestrator tests for `--from xb10 --to callback`, including the unchanged Gateway path and unsupported-source rejection before active emission.
- [x] 3.3 Add or extend an opt-in Podman smoke for one XB10-to-event-sink correlated diagnostic event; require explicit active-event consent and assert capability discovery, exactly one source acceptance, routing observation, and matching receipt.

## 4. Operator Contract And Verification

- [x] 4.1 Update `docs/health.md`, `docs/runbook.md`, and `docs/cpe-webpa-callback-diagnostic.md` to list Gateway and XB10 as supported callback sources, include an XB10 invocation, and retain the distinction from Parodus client inventory.
- [x] 4.2 Run focused Go diagnostic, type-registration, health-daemon, CLI/help, and race tests, plus `go build ./cmd/vcpe`; record unrelated repository-wide failures separately.
- [x] 4.3 Build and deploy the XB10 image, run the opt-in active callback smoke against the packaged simulator, then run strict OpenSpec validation and `git diff --check`.
## 1. Diagnostic Model And Talaria Collection

- [x] 1.1 Extend `controlplane/internal/diagnostic/model.go` with the `talaria-devices` journey, 64-device bound, validated `TalariaDevice`/`TalariaDevices` endpoint and result fields, and journey-specific empty-invocation validation.
- [x] 1.2 Extend `controlplane/internal/diagnostic/safety.go` and `render.go` so device lists are defensively copied, ID-sorted, emitted only for the Talaria inventory journey, and rendered with explicit populated and empty states in ASCII and JSON.
- [x] 1.3 Add a WebPA-local Talaria inventory operation to `controlplane/internal/diagnostic/cpewebpa.go` or a focused sibling probe that performs one bounded authenticated `GET /api/v2/devices`, translates only the specified session fields, and maps transport, authentication, status, decode, validation, and over-limit outcomes to bounded observations without partial entries.
- [x] 1.4 Add model, safety, renderer, and Talaria-probe tests for valid multi-device and empty inventories, ID ordering, raw operator-visible IDs and counters, malformed fields, oversized responses, 65-session limits, credentials/non-selected fields exclusion, and passive GET-only behavior.

## 2. WebPA-Local Journey Wiring

- [x] 2.1 Add a WebPA-to-Talaria `talaria-devices` provider and resolver target mapping in `controlplane/internal/diagnostic/provider.go` and `resolver.go`; select only the persisted WebPA source endpoint and require `--replica` for multi-replica sources.
- [x] 2.2 Extend the diagnostic client and orchestrator to invoke, validate, and copy `talariaDevices` from the selected WebPA endpoint while preserving existing CPE, webhook, callback, Argus inventory, and Parodus journeys.
- [x] 2.3 Register and dispatch the new capability in `controlplane/cmd/vcpe-healthd`, `controlplane/internal/types`, and `services/webpa/container/entrypoint.sh` without adding device-directed or Talaria-control traffic.
- [x] 2.4 Add provider, resolver, client, orchestrator, health-daemon, and type-registration tests for WebPA-only capability discovery, persisted-loopback-only collection, multi-replica selection, successful empty and populated responses, and bounded source-local failures.

## 3. CLI Contract And Output

- [x] 3.1 Extend `controlplane/internal/app/cli.go` so `vcpe diag` and `vcpe diagnose` accept `--to devices` with common deployment/source/replica/JSON behavior, and reject client-service, subscriber, subscriber-replica, active-callback, active-event, event, and device-id flags before state access.
- [x] 3.2 Update `controlplane/internal/app/help.go` and help goldens to document the passive WebPA-source device-session inventory, raw operator-visible session data, empty-list semantics, 64-device bound, and its distinction from selected-device `--to webpa` diagnosis.
- [x] 3.3 Add CLI parsing and `executeLocal` tests for the `diag --from webpa --to devices` alias form, every cross-journey rejection, persisted WebPA loopback invocation, and human/JSON structured device-session output.

## 4. Verification

- [x] 4.1 Run focused `go test ./internal/diagnostic`, `go test ./cmd/vcpe-healthd`, and `go test ./internal/app` coverage from `controlplane/`; recorded unrelated app failures: absent staged `services/bng/container/vcpe-healthd`, occupied `127.0.0.1:47000`, and the `development` branch release expectation.
- [x] 4.2 Ran `gofmt` on touched Go files, `go test ./...`, and `openspec validate webpa-device-inventory --strict` (passed); full module failures remain limited to the known missing staged BNG health daemon, occupied `127.0.0.1:47000`, and `development` branch release expectation.
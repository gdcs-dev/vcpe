## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| RBUS method self-registration | `services/gateway/container/elements.json` — no changes needed (architecture.md incorrectly stated elements.json would be modified) | No |
| Argus registration via ancla v0.3.12 | Webhook registration uses `ancla.NewService()` + `svc.Add()` with `ancla.InternalWebhook`, NOT raw Argus PUT / chrysom / ManifestV1 | No |
| event-sink is a registered vCPE service type | `controlplane/internal/types/eventsink/`, `types.go`, orchestrator teardown list | No |

> **NOTE:** Several planning-phase decisions below were superseded during
> implementation. See the **As-Built Corrections** section at the end of this
> document for the authoritative final state. The superseded decisions are
> retained for the historical record.

---

## Decisions

### Decision: apparmor-simulator source location
Recommendation: In-tree under `services/gateway/src/apparmor-simulator/`, built via `COPY` in the Containerfile builder stage
Decision: Proceed with recommended approach
Rationale: First-party simulator code belongs in-tree. The `git clone` pattern is for third-party upstream C dependencies. In-tree keeps simulator changes and Containerfile changes in the same commit, eliminates network fetches at build time, and produces reproducible images.

Q: Where should the apparmor-simulator source code live?
A: In-tree, `services/gateway/src/apparmor-simulator/`

---

### Decision: Timer interval configuration
Recommendation: Environment variable `APPARMOR_SIM_INTERVAL_SEC`, default 30
Decision: Proceed with recommended approach
Rationale: The interval is a startup-time config value, not a live operational parameter. Env vars are the established vCPE pattern for configuring compose services. RBUS data element would add boilerplate for no meaningful benefit; compile-time constant is too rigid for a test harness.

Q: How should the emission interval be configured?
A: Environment variable `APPARMOR_SIM_INTERVAL_SEC`

---

### Decision: AppArmor event fixture data storage
Recommendation: Hardcoded C arrays (profiles, operations, targets defined in source)
Decision: Proceed with recommended approach
Rationale: The simulator's job is to generate plausible-looking events. A JSON config file would add cjson parsing boilerplate, a COPY step, and a class of startup failure (missing/malformed file). Hardcoded arrays keep the C source self-contained. Event set changes are gated by a rebuild, which is appropriate.

Q: How should the event fixture data be stored?
A: Hardcoded C arrays in the source

---

### Decision: event-sink Go module structure
Recommendation: Separate Go module at `services/event-sink/go.mod`, module path `github.com/gdcs-dev/vcpe/event-sink`
Decision: Proceed with recommended approach
Rationale: The controlplane module is the `vcpe` operator CLI. The event-sink is a long-running server binary with different dependencies (HTTP server, XMiDT libraries). Coupling them would risk breaking `go build ./...` and `make release-gate` on unrelated dependency bumps.

Q: How should the event-sink Go module be structured?
A: Separate module at `services/event-sink/go.mod`

---

### Decision: compose.yaml placement for event-sink
[OVERRIDE]
Recommendation: Add directly to `services/webpa/compose.yaml` as a companion service
Decision: Own compose file at `services/event-sink/compose.yaml`
Rationale: Follows the established per-service compose.yaml pattern demonstrated by `services/oktopus/compose.yaml`. Each vCPE service owns its directory and compose definition.
Override reason: Consistency with existing repo layout convention. The operational constraint (must be started with `--env-file ../webpa/compose.env` for `${IFACE_MGMT_NETWORK}` substitution) is accepted.

Operational note: The event-sink compose must be started with:
```
docker compose --env-file ../webpa/compose.env -f services/event-sink/compose.yaml up -d
```
The `env_file:` service directive only injects container env vars; compose-file variable substitution requires `--env-file` or a local `.env` file. The `image:` field is hardcoded as `ghcr.io/gdcs-dev/vcpe/event-sink:dev`.

Q: Where should the event-sink compose service be defined?
A: Own `services/event-sink/compose.yaml`, following the oktopus pattern

---

### Decision: Argus credentials provisioning
Recommendation: Env vars directly in `compose.yaml`
Decision: Proceed with recommended approach
Rationale: These are dev-only credentials already present in plaintext in `argus.yaml` and other config files in the repo. No new secret exposure. A `.env` file adds gitignore ceremony and manual provisioning with no security benefit for a dev/test tool.

Q: How should Argus credentials be provisioned to the event-sink?
A: Env vars `ARGUS_BASIC_AUTH` and `WEBHOOK_SECRET` in `compose.yaml`

---

### Decision: RBUS method ownership
[BREAKING]
Recommendation: apparmor-simulator self-registers `Device.AppArmor.SimulateEvent()` via `rbus_regDataElements()` with a `methodHandler` callback. No `elements.json` changes.
Decision: Proceed with recommended approach (and corrects architecture.md error)
Rationale: `rbus-elements` registers static data elements for property get/set and event publication. It cannot proxy an active method call into a different running process. `architecture.md` incorrectly stated that `elements.json` should be modified — this was an error. Only the process that will handle the method invocation can register it. The `parodus2rbus` pattern demonstrates self-registration.

Q: Who owns and handles the `Device.AppArmor.SimulateEvent()` RBUS method?
A: apparmor-simulator self-registers via `rbus_regDataElements()`. No elements.json changes.

---

### Decision: event-sink container base image
Recommendation: Multi-stage — `golang:1.24-alpine` builder → `alpine:3` runtime
Decision: Proceed with recommended approach
Rationale: Multi-stage is the canonical Go container pattern. `alpine:3` runtime (over distroless) is chosen for the shell availability (`docker exec` debugging) appropriate to a dev/test tool.

Q: What base image should the event-sink container use?
A: Multi-stage: `golang:1.24-alpine` builder → `alpine:3` runtime

---

### Decision: Argus TTL-refresh implementation
[BREAKING]
Recommendation: `ancla` library (`github.com/xmidt-org/ancla`)
Decision: `ancla/chrysom` subpackage as Argus HTTP client + `time.NewTicker` for TTL refresh
Rationale: `ancla` (top-level package) is a server-side webhook registry library for XMiDT services (Caduceus) — it is NOT a client-side registration tool. The event-sink is a webhook registrant, not a registry. The correct approach is `ancla/chrysom` (the Argus HTTP client) to construct and PUT the webhook item with the format Caduceus expects, plus a `time.NewTicker(6 * time.Hour)` goroutine for TTL refresh at TTL/2.

Q: How should the Argus TTL-refresh be implemented?
A: `ancla/chrysom` as Argus client + manual ticker goroutine

---

### Decision: Testing strategy
Recommendation: Go unit tests for webhook handler (HMAC validation, malformed payload) and Argus registration path using `httptest.Server` mocks. No C tests.
Decision: Proceed with recommended approach
Rationale: The C simulator is thin IPC glue over libparodus/RBUS — minimal standalone logic worth unit-testing. The Go logger has well-defined contract boundaries (HTTP handler, HMAC signature validation, JSON parsing) that map cleanly to `httptest`-based tests. HMAC validation in particular must be tested to prevent silent security bypass.

Q: What is the testing strategy?
A: Go unit tests for webhook handler and Argus registration path only. No C tests.

---

### Decision: libparodus client URL port
Recommendation: `tcp://127.0.0.1:6670`
Decision: Proceed with recommended approach
Rationale: `parodus2rbus` uses port 6668. Port 6670 is unallocated and follows the even-numbered port pattern. Note: the `parodus_url` (where the simulator connects to parodus) remains `tcp://127.0.0.1:6666` — the `client_url` (6670) is a separate `libpd_cfg_t` field for the simulator's own receive endpoint.

Q: What port should the apparmor-simulator use for its libparodus client URL?
A: `tcp://127.0.0.1:6670`

---

### Decision: Systemd service dependency on parodus
Recommendation: `After=parodus.service` only (no `Requires=`)
Decision: Proceed with recommended approach
Rationale: Consistent with `parodus2rbus.service` and `parodus.service` patterns in the gateway container. The retry loop in the simulator handles parodus restarts. Adding `Requires=` would cause unnecessary systemd-driven restarts on every parodus cycle, generating log noise and losing timer state.

Q: What systemd dependency should `apparmor-simulator.service` declare on parodus?
A: `After=parodus.service` only

---

### Decision: MAC address source in apparmor-simulator
[OVERRIDE]
Recommendation: Environment variable `GATEWAY_MAC_ADDRESS` set by the systemd unit via `ExecStartPre`
Decision: Direct sysfs read from `/sys/class/net/erouter0/address` in C at startup
Rationale: Consistent with how `start_parodus.sh` already reads the MAC. The interface name `erouter0` is an established assumption for this container. Direct file I/O is simple C with no extra systemd unit configuration.
Override reason: The sysfs pattern is already used and `erouter0` is a fixed assumption for the gateway container. Avoiding the extra `ExecStartPre` complexity is preferred.

Implementation note: C code reads `/sys/class/net/erouter0/address`, strips colons with a simple loop. Error path: if open fails, log ERROR and exit (systemd `Restart=always` handles recovery when the interface comes up).

Q: How should the device MAC be made available to the apparmor-simulator C code?
A: Direct sysfs read from `/sys/class/net/erouter0/address` in C

---

### Decision: Event destination severity segment
Recommendation: Include a severity segment in the WRP event dest: `event:apparmor/{severity}/mac:{device_mac_hex}`
Decision: Proceed with recommended approach
Rationale: The three-segment format gives Caduceus-level filtering by severity without payload inspection. The `apparmor` field in the JSON payload (e.g., `DENIED`) lowercased becomes the severity path segment. The webhook regex `apparmor/.*` matches all severities; `apparmor/denied.*` matches only violations. The WRP `event:` scheme treats the full post-colon value as an opaque string — path segments are convention only.
Severity values: `denied` (DENIED), `audit` (AUDIT), `allowed` (ALLOWED). The hardcoded C event fixture array maps each simulated event to one of these values.

Q: Does XMiDT support multi-segment event destinations like `event:apparmor/alert/mac:{device}`?
A: Yes — updated to `event:apparmor/{severity}/mac:{device_mac_hex}`. Webhook regex updated from `apparmor.*` to `apparmor/.*`.

---

### Decision: event-sink webhook configurability
Recommendation: `WEBHOOK_EVENTS_REGEX` and `WEBHOOK_DEVICE_MATCHER` env vars with defaults `apparmor/.*` and `.*`
Decision: Proceed with recommended approach
Rationale: The service is renamed from `apparmor-logger` to `event-sink` to reflect that it can capture any WRP event stream, not just AppArmor events. The events regex and device matcher are the only two parameters needed to retarget the sink. Defaults are set for the AppArmor simulator use case so no configuration change is needed to use it with the apparmor-simulator out of the box. To spy on a different event type, change `WEBHOOK_EVENTS_REGEX` in the compose service definition and restart.

Additional env var: `WEBHOOK_SECRET` (renamed from `APPARMOR_WEBHOOK_SECRET`) — the HMAC secret for validating Caduceus webhook POSTs.

Q: Should the event-sink be renamed and made generic?
A: Yes — renamed to `event-sink`, events regex configurable via `WEBHOOK_EVENTS_REGEX` (default `apparmor/.*`), device matcher via `WEBHOOK_DEVICE_MATCHER` (default `.*`).

---

### Decision: C build system for apparmor-simulator
Recommendation: CMake, following the established pattern for all C dependencies in the Containerfile
Decision: Proceed with recommended approach
Rationale: Every C dependency in the gateway Containerfile uses CMake. The `CMakeLists.txt` links: `wrp-c`, `libparodus` (via nanomsg), `rbus`, `cjson`, `pthread`. `COPY src/apparmor-simulator /opt/git/apparmor-simulator` in the builder stage, then standard `cmake CMakeLists.txt -DCMAKE_INSTALL_PREFIX=$INSTALL_PREFIX && make && make install`.

Q: (Domain checklist — build system)
A: CMake, matching existing Containerfile pattern

---

### Decision: compose variable substitution mechanism
Recommendation: Run with `docker compose --env-file ../webpa/compose.env -f compose.yaml up`
Decision: Proceed with recommended approach
Rationale: `env_file:` under a service definition injects container environment variables; it does NOT provide variables for compose-file interpolation (`${IFACE_MGMT_NETWORK}` in the `networks:` block). The `--env-file` flag is required for interpolation. This is an accepted operational constraint for a dev/test tool not managed by the vCPE control plane.

Q: (Domain checklist — deployment)
A: `--env-file ../webpa/compose.env` flag on startup

---

## As-Built Corrections

The following reflects what was actually built and verified working end-to-end.
Where these conflict with a planning decision above, **these are authoritative**.

### AB1: Argus registration uses ancla v0.3.12 `InternalWebhook` + `svc.Add()`
**Supersedes:** "Argus TTL-refresh implementation" (chrysom direct) and any ManifestV1/raw-PUT approach.
The deployed Caduceus/tr1d1um use `ancla v0.3.12` + `argus/chrysom` (from `argus v0.9.10`). The event-sink registers via `ancla.NewService(ancla.Config{...})` then `svc.Add(ctx, "", ancla.InternalWebhook{...})`. Hand-rolled Argus PUTs failed silently: earlier attempts wrote a flat webhook `data` blob, then an ancla-v0.4.8-style `ManifestV1` (`wrp_event_stream_schema_v1`), neither of which the deployed ancla v0.3.12 could deserialize — Caduceus created zero senders. Matching the exact library + version that Caduceus reads with is mandatory.
- `ancla.Config`: `JWTParserType: "simple"`, `DisablePartnerIDs: true`, `BasicClientConfig{Address, Bucket: "webhooks", Auth.Basic}`
- `InternalWebhook.Webhook`: `Config.URL`, `Config.ContentType: "application/json"`, `Config.Secret`, `Events`, `Matcher.DeviceID`, `Duration`, `Until`
- Refresh: re-`Add()` on a 6h `time.Ticker`.

### AB2: HMAC is SHA1, not SHA256
**Supersedes:** all references to HMAC-SHA256.
ancla's `DeliveryConfig.Secret` is documented as "the string value for the SHA1 HMAC." Caduceus signs webhook POST bodies with `X-Webpa-Signature: sha1=<hex>`. The handler validates with `crypto/sha1` and the `sha1=` prefix. The `Config.Secret` MUST be set in the registration or Caduceus sends unsigned requests.

### AB3: Events regex must NOT include the `event:` scheme prefix
**Supersedes:** the "Event destination severity segment" note that used `event:apparmor/.*`.
Caduceus strips the `event:` scheme from the WRP destination before matching webhook event regexes. For a dest of `event:apparmor/denied/mac:...`, Caduceus matches against `apparmor/denied/mac:...`. The correct `WEBHOOK_EVENTS_REGEX` is `apparmor/.*` (verified: `event:apparmor/.*` produces "destination regex doesn't match"). The device MAC/simulator still emits `dest = event:apparmor/{severity}/mac:{mac}`.

### AB4: event-sink receives WRP metadata in HTTP headers, payload in body
**Supersedes:** the assumption that the webhook body is a JSON WRP envelope.
Caduceus delivers the WRP payload as the raw HTTP body and the WRP metadata as `X-Xmidt-*`/`X-Webpa-*` headers. The handler reconstructs the message with `wrphttp.SetMessageFromHeaders(r.Header, &msg)` and sets `msg.Payload = body`, with a fallback to decoding the body as a msgpack/JSON WRP message. (`json.Unmarshal` and msgpack-body decode both silently produced empty structs.) Adds dependency `github.com/xmidt-org/wrp-go/v3`.

### AB5: apparmor-simulator logs via syslog, not cimplog
**Supersedes:** "Observability Strategy" cimplog references.
The C daemon uses `openlog()`/`syslog(LOG_INFO|LOG_ERR, ...)` (facility `LOG_DAEMON`). `cimplog` remains only as a **transitive link dependency** of `libwrp-c.so` (provides `__cimplog`) — it must stay in the CMake link list but is not called directly.

### AB6: event-sink is a registered vCPE service type (not a standalone/companion compose)
**Supersedes:** "compose.yaml placement" and "compose variable substitution mechanism".
A new curated service type `event-sink` is registered in `controlplane/internal/types/eventsink/` (mirrors the webpa type: no typed config, renders `compose.env` via `render.IfaceEnv`). The control plane generates `services/event-sink/compose.env` during `vcpe apply` (same as webpa/oktopus), so `compose.yaml` uses `env_file: compose.env` and `${IFACE_*}` interpolation works without a manual `--env-file` flag. The manifest entry is `type: event-sink`. Orchestrator teardown candidate list and `stage-runtime-init-binaries` known-services list both include `event-sink`.

### AB7: runtime-init-event-sink binary
The container follows the vCPE init pattern: `ENTRYPOINT ["/usr/local/bin/runtime-init-event-sink"]` + `CMD ["/usr/local/bin/entrypoint.sh"]`. Source at `controlplane/cmd/runtime-init-event-sink/main.go`; committed binary staged by `scripts/stage-runtime-init-binaries event-sink`.

### AB8: Container base image is `debian:bookworm-slim`
**Supersedes:** "event-sink container base image" (alpine).
Builder stays `golang` multi-stage; runtime is `debian:bookworm-slim` with `iproute2`, `isc-dhcp-client`, `ca-certificates`, etc. The entrypoint runs `dhclient` on `eth0` to obtain the management IP from BNG's dnsmasq DHCP server — this registers the container hostname in BNG dnsmasq so peers (Caduceus) resolve `event-sink` by name. (The `BNG_DNS_SERVER`/static-resolv.conf approach was replaced by DHCP.)

### AB9: Argus endpoint path is `/api/v1/store/{bucket}/{id}`
The Argus store API is under `/api/v1/store/...` (not `/store/...`). ancla/chrysom handles this internally; noted for direct-curl debugging. Argus is reached at `http://webpa:6600` (runs inside the webpa container).


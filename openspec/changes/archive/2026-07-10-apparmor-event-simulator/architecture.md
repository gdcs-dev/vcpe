# Architecture: AppArmor Event Simulator

## Overview

This change adds two new components to the vCPE system:

1. **apparmor-simulator** — a C daemon running inside the existing gateway container that simulates AppArmor security audit events and delivers them to the XMiDT cloud via the already-running parodus agent using WRP EVENT messages.

2. **event-sink** — a Go service that runs alongside the existing XMiDT stack (webpa/caduceus/argus), registers a configurable webhook via Argus, and logs all matching WRP events delivered to it by Caduceus.

The feature exercises the full XMiDT event pipeline end-to-end: device C client → libparodus → parodus → Talaria (webpa) → Caduceus → event-sink webhook.

```
Gateway Container                   XMiDT Stack (webpa network)
─────────────────────               ─────────────────────────────────
apparmor-simulator (C)              event-sink (Go)
    │  libparodus                        │  HTTP :8080/webhook
    ▼                                    │
parodus ──── WebSocket ────▶ talaria     │
                                │        │
                                ▼        │
                            caduceus ───▶│ (POST event)
                                │
                                ▼
                             argus  ◄─── event-sink (PUT /store/webhooks/{id})
                          (webhook store)
```

## Components

### apparmor-simulator (device-side C daemon)
Runs inside the gateway container as a systemd service. Registers with the local parodus process using `libparodus`. On a configurable timer (default 30s) emits `WRP_MSG_TYPE__EVENT` messages with a JSON payload that mimics a real AppArmor audit log entry. Simultaneously registers an RBUS method `Device.AppArmor.SimulateEvent()` that can be called on-demand to trigger immediate emission. Both trigger paths use the same event emission function.

### event-sink (cloud-side Go service)
Runs as a container on the same compose network as the webpa/caduceus/argus stack. On startup, registers a webhook with Argus using the `ancla` library (v0.3.12, `ancla.NewService` + `svc.Add` with an `ancla.InternalWebhook`) — matching the ancla version the deployed Caduceus reads with. Configurable events regex (`WEBHOOK_EVENTS_REGEX`, default `apparmor/.*` — no `event:` prefix, which Caduceus strips before matching) and device matcher (`WEBHOOK_DEVICE_MATCHER`, default `.*`). Exposes an HTTP server on port 8080 to receive Caduceus event POSTs. Validates the `X-Webpa-Signature` **HMAC-SHA1** header (`sha1=<hex>`) before processing. Reconstructs the WRP message from `X-Xmidt-*`/`X-Webpa-*` headers (payload in the body) via `wrphttp.SetMessageFromHeaders`. Logs each received WRP event as structured JSON to stdout. Re-registers via `svc.Add` on a 6h ticker to keep the Argus item alive. Registered as a vCPE service type `event-sink` whose `compose.env` is rendered by `vcpe apply`; obtains its management IP via DHCP from BNG dnsmasq so the hostname resolves for delivery.

### gateway/Containerfile additions
Build stage: no new dependencies required — `libparodus`, `libwrp-c`, `librbus`, and `libcjson` are all already compiled and staged. Source lives in-tree at `services/gateway/src/apparmor-simulator/`; a `COPY src/apparmor-simulator /opt/git/apparmor-simulator` step followed by CMake builds the daemon. Runtime stage: adds the compiled binary and an `apparmor-simulator.service` unit and enables it. No changes to `elements.json` — the simulator self-registers `Device.AppArmor.SimulateEvent()` via `rbus_regDataElements()` at startup.

## Key Architectural Decisions

### Device-side service language: C via libparodus
**Choice**: C, using the `libparodus` API directly, following the `parodus2rbus` pattern.
**Rationale**: Every existing gateway service is C. All required libraries (libparodus, wrp-c, rbus, cjson) are already compiled and installed in `$INSTALL_PREFIX`. Adding Go would introduce a runtime of its own, increase image size by 10–20 MB, and break the established pattern with no meaningful benefit for this scope.
**Alternatives considered**: Go — rejected because it has no client-side XMiDT library equivalent to libparodus and would require a CGO bridge or a reimplementation of the WRP framing protocol. Shell script — rejected because it cannot maintain a persistent libparodus connection or register RBUS methods.

### WRP message type: EVENT (not REQ)
**Choice**: `WRP_MSG_TYPE__EVENT` for all simulator emissions.
**Rationale**: AppArmor telemetry is fire-and-forget. Events require no acknowledgement and are the correct WRP type for unidirectional device→cloud telemetry. REQ/RESP would require the cloud to send a matching reply, adding unnecessary complexity.
**Alternatives considered**: REQ — rejected because it implies bidirectional conversation, blocking the sender until a response timeout, with no benefit for telemetry.

### Event source/destination format
**Choice**:
- `source`: `mac:{device_mac_hex}/apparmor-simulator`
- `dest`: `event:apparmor/{severity}/mac:{device_mac_hex}`

Where `{severity}` is the lowercase AppArmor disposition: `denied`, `audit`, or `allowed`.

**Rationale**: The three-segment dest path `apparmor/{severity}/mac:{device}` gives downstream consumers fine-grained filtering. A consumer wanting all AppArmor events uses regex `apparmor/.*`; one wanting only violations uses `apparmor/denied.*`; one scoped to a single device uses `apparmor/.*/mac:aabbccddeeff`. The WRP `event:` scheme treats everything after the colon as an opaque string matched against regexes — path segments are just convention, not protocol structure.
**Alternatives considered**: `event:apparmor/mac:{device}` (no severity) — rejected because it forces consumers to parse the payload to filter by severity rather than using Caduceus-level regex filtering. `event:apparmor` (no scoping) — rejected because it loses device identity.

### Dual-trigger: unified daemon (not systemd timer unit)
**Choice**: A single long-running daemon uses `timer_create`/`timerfd` for the interval trigger and a registered RBUS method for on-demand trigger. Both paths call the same internal `emit_apparmor_event()` function.
**Rationale**: A systemd `.timer` + oneshot `.service` pair would require `rbuscli` to invoke the running daemon's RBUS method externally, adding a third systemd unit, a process-spawn overhead per tick, and a fragile dependency on `rbuscli` availability in the exec context. A unified daemon is simpler and matches the `rbus-elements` / `parodus2rbus` pattern.
**Alternatives considered**: Systemd timer + oneshot binary — rejected (see rationale). Separate timer process + method process — rejected because two independent parodus clients for the same service name would conflict.

### Argus URL: `http://webpa:6600`
**Choice**: The event-sink connects to Argus at `http://webpa:6600`.
**Rationale**: Argus is not a standalone container. It is started by the webpa container's entrypoint and binds to port 6600 on localhost inside that container, but webpa exposes port 6600 externally on the compose network. There is no `argus` DNS name.
**Alternatives considered**: `http://argus:6600` — rejected, DNS fails at runtime. `http://127.0.0.1:6600` — rejected, only valid inside the webpa container.

### Webhook registration: Argus-backed (persistent)
**Choice**: event-sink registers its webhook via Argus `PUT /store/webhooks/{sha256(url)}` with TTL=12h, refreshed at TTL/2 via a background goroutine using `ancla/chrysom` (the Argus HTTP client subpackage) and a `time.NewTicker(6h)` loop. Note: the top-level `ancla` package is a server-side webhook registry for Caduceus and is not used here.
**Rationale**: Direct Caduceus `/hook` registration is ephemeral — it disappears when Caduceus restarts. Argus-backed registration survives Caduceus restarts because Caduceus reads its webhook list from Argus. This is the production-intended pattern for durable webhook registration in XMiDT.
**Alternatives considered**: Direct Caduceus `/hook` endpoint — rejected because it is transient; any Caduceus restart loses the registration, breaking event delivery until the event-sink re-registers (which requires the event-sink to detect the outage). The Argus-backed approach is self-healing.

### HMAC secret: pre-provisioned stable env var
**Choice**: The HMAC secret is injected into event-sink as an environment variable `WEBHOOK_SECRET`. It is also embedded in the Argus webhook item `data.config.secret` so Caduceus can sign POSTs. The secret never changes at runtime.
**Rationale**: The Argus item ID is `sha256(webhook_url)`. If the secret changes, the Argus item ID remains the same (it is keyed on URL), so a new PUT with the updated secret simply overwrites the old item — no stale state. The event-sink must load the secret before the first PUT to Argus to ensure Caduceus receives the current value.
**Alternatives considered**: Dynamic secret rotation — deferred as out of scope for a simulator; the static secret approach is acceptable for dev/test use.

### Cloud service location: `services/event-sink/`
**Choice**: The Go service lives in `services/event-sink/` alongside `services/webpa/`, `services/gateway/`, etc.
**Rationale**: All vCPE containerized services are siblings in `services/`. No architectural distinction between "device" and "cloud" containers at the repo layout level — both are services the vCPE harness manages.
**Alternatives considered**: `cloud-services/` — rejected; adds a new top-level directory pattern with no benefit given existing conventions.

### Network attachment: standalone compose with env-file reference
**Choice**: event-sink has its own `services/event-sink/compose.yaml`, following the same per-service pattern as `services/oktopus/`. It uses `${IFACE_MGMT_NETWORK}` from `services/webpa/compose.env` via the `--env-file` flag at startup: `docker compose --env-file ../webpa/compose.env -f compose.yaml up -d`. The `image:` field is hardcoded as `ghcr.io/gdcs-dev/vcpe/event-sink:dev`.
**Rationale**: Each vCPE service owns its `services/<name>/compose.yaml` directory — this is the established pattern (oktopus, gateway, webpa). Adding event-sink to `services/webpa/compose.yaml` would conflate two logically separate services. The `--env-file` mechanism provides `${IFACE_MGMT_NETWORK}` for compose-file variable substitution without requiring a registered service type.
**Alternatives considered**: Add directly to `services/webpa/compose.yaml` — rejected, breaks per-service ownership convention. New vCPE service type — deferred; appropriate long-term but conflicts with the in-flight manifest-driven-redesign.

## Data Flow

### Event emission (device → cloud)

```
apparmor-simulator daemon
    │
    │  timer fires (30s) OR RBUS method invoked
    │
    ▼
emit_apparmor_event()
    │  constructs WRP_MSG_TYPE__EVENT
    │    source: "mac:aabbccddeeff/apparmor-simulator"
    │    dest:   "event:apparmor/denied/mac:aabbccddeeff"  (severity from payload apparmor field)
    │    payload: JSON AppArmor event (see schema below)
    │
    ▼
libparodus_send(inst, wrp_msg)
    │
    │  nanomsg IPC  tcp://127.0.0.1:6666
    ▼
parodus (gateway container)
    │
    │  WebSocket (wss://)
    ▼
Talaria (webpa container, port 6200)
    │
    │  internal XMiDT routing
    ▼
Caduceus (webpa container)
    │  matches event against registered webhooks
    │  webhook: {url: "http://event-sink:8080/webhook", events: ["${WEBHOOK_EVENTS_REGEX}"]}
    │  signs body with HMAC-SHA256 → X-Webpa-Signature header
    │
    │  HTTP POST
    ▼
event-sink :8080/webhook
    │  validates X-Webpa-Signature
    │  parses JSON payload
    ▼
structured log output (stdout/stderr)
```

### Webhook registration (startup)

```
event-sink starts
    │
    ├──▶ PUT http://webpa:6600/store/webhooks/{sha256("http://event-sink:8080/webhook")}
    │    Authorization: Basic <argus-credentials>
    │    Body: { id, data: { config: { url, events: ["${WEBHOOK_EVENTS_REGEX}"], content_type, secret } }, ttl: 43200 }
    │
    │    Argus persists item
    │
    ▼
background goroutine (TTL refresh at 6h intervals)
    │
    └──▶ PUT (same URL, same body) → keeps item alive in Argus
```

## AppArmor Event JSON Schema

```json
{
  "timestamp": "2026-07-09T10:30:00Z",
  "device_id":  "mac:aabbccddeeff",
  "apparmor": "DENIED",
  "operation": "open",
  "profile":  "/usr/sbin/dnsmasq",
  "name":     "/etc/shadow",
  "pid":      1234,
  "comm":     "dnsmasq",
  "requested_mask": "r",
  "denied_mask":    "r",
  "fsuid":    33,
  "ouid":     0,
  "simulated": true
}
```

The `simulated: true` field distinguishes simulator events from future real events. Operations and profiles are drawn from configurable tables seeded with realistic gateway-service patterns (dnsmasq, nginx, parodus, etc.).

## Integration Points

| Point | Direction | Protocol | Notes |
|-------|-----------|----------|-------|
| parodus local URL `tcp://127.0.0.1:6666` | simulator → parodus | nanomsg | apparmor-simulator registers as service `apparmor-simulator` |
| parodus → Talaria | parodus → cloud | WebSocket/WRP | existing connection; simulator adds event traffic |
| Caduceus `PUT /hook` (via Argus) | event-sink → argus | HTTP | argus at `http://webpa:6600` |
| Caduceus → webhook | caduceus → event-sink | HTTP POST | event-sink at `http://event-sink:8080/webhook` |
| RBUS method `Device.AppArmor.SimulateEvent()` | external → simulator | RBUS IPC | optional test trigger; self-registered by apparmor-simulator via `rbus_regDataElements()` |

## Security Model

**Device side:**
- apparmor-simulator runs as root (same as all gateway container services) — acceptable in the dev/test context; no privilege escalation vector exists since everything in the container is already root.
- Parodus local URL (`tcp://127.0.0.1:6666`) is loopback-only. No remote access to the IPC bus.
- WRP messages carry no authentication credentials; Parodus is trusted to relay them. The device identity is carried by the parodus MAC binding (`--hw-mac`), which is established at parodus startup.

**Cloud side:**
- All Argus writes require `Authorization: Basic` credentials, injected via environment variable `ARGUS_BASIC_AUTH`. Never hardcoded.
- Caduceus signs event POSTs with `X-Webpa-Signature: sha256=<hmac>`. event-sink MUST validate this signature before processing any payload. Unsigned requests are rejected 401.
- The HMAC secret (`WEBHOOK_SECRET`) is injected via environment variable; not logged, not stored in plaintext in any state file.
- event-sink's `/webhook` endpoint is on the internal compose network only. It is not exposed on any host port and is not reachable from outside the compose network.
- All inter-service communication within the XMiDT stack is within the compose network; no TLS required for internal calls in the dev environment.

**Trust boundary summary:**
```
[ Internet / external ]
        │
        │  (no exposure)
[ compose mgmt network ]
   webpa:6200   (talaria - device connections, TLS)
   webpa:6600   (argus   - internal only)
   caduceus     (internal only)
   event-sink:8080  (internal only, caduceus-only consumer)
```

## Error Handling Strategy

**apparmor-simulator (C):**
- `libparodus_init` failure: log error, retry with exponential backoff (up to 60s), then sleep-loop. Never exit — systemd `Restart=always` handles persistent failures.
- `libparodus_send` failure: log warning, drop event (telemetry loss is acceptable for a simulator). Do not block the timer loop.
- RBUS registration failure: log warning, continue without RBUS method. Timer-based emission still functions.

**event-sink (Go):**
- Argus registration failure on startup: retry with exponential backoff (1s, 2s, 4s … max 30s) indefinitely. The service is not useful without a registration; it must not give up.
- Argus TTL refresh failure: log error, retry at next interval. Do not crash.
- Invalid HMAC signature on incoming webhook: log warning with source IP, return HTTP 401. Do not process payload.
- Malformed JSON payload: log error with raw body (truncated to 1KB), return HTTP 400.
- HTTP server errors: let the Go http.Server handle them; log at ERROR.

## Observability Strategy

**apparmor-simulator (C):**
- Uses `syslog` (facility `LOG_DAEMON`, opened via `openlog`) for structured logging — visible in the gateway container's syslog with the `apparmor-simulator[pid]:` prefix. (`cimplog` remains only as a transitive link dependency of `libwrp-c`.)
- Log levels:
  - `LOG_INFO`: service start, parodus registration success, each event emitted (dest, operation, profile)
  - `LOG_ERR`: parodus send failure, RBUS registration failure, MAC read failure, fatal configuration errors

**event-sink (Go):**
- Uses `log/slog` (stdlib, no extra dependency) with JSON handler writing to stdout.
- Log levels:
  - `INFO`: service start, webhook registered in Argus (events_regex, device_matcher), webhook TTL refreshed, event received (dest, device_id, content_type, payload_size)
  - `WARN`: HMAC validation failure, malformed payload, Argus refresh failed
  - `ERROR`: Argus registration failed (with retry count), HTTP server error
- No custom metrics in v1 (out of scope for a simulator). A `/health` endpoint returning `200 OK` is sufficient for basic liveness probing.

## Constraints

- **apparmor-simulator must not introduce new build-stage dependencies.** All required libraries (libparodus, wrp-c, rbus, cjson) are already installed in `$INSTALL_PREFIX`. The Containerfile build chain is already long; new `git clone` + build blocks are costly in CI.
- **apparmor-simulator must not conflict with the `apparmor-simulator` service name in parodus.** Service names must be unique per parodus instance. The name `apparmor-simulator` is reserved by this design; no other service may register it.
- **RBUS element path must follow `Device.` namespace convention.** `Device.AppArmor.SimulateEvent()` follows the TR-069/TR-181 naming scheme used throughout rbus-elements.
- **event-sink must not be registered as a vCPE service type during the manifest-driven-redesign.** The typeregistry is mid-rewrite. The standalone compose pattern (`services/event-sink/compose.yaml` with `--env-file ../webpa/compose.env`) avoids conflicting with task sequencing in `openspec/changes/manifest-driven-redesign/tasks.md`.
- **Webhook registration must use the `webhooks` bucket in Argus.** Caduceus is hard-configured to read from `bucket: webhooks`. Using any other bucket name will result in Caduceus never discovering the webhook.
- **TTL must not exceed Argus `itemMaxTTL` (24h in current config).** Chosen TTL of 12h is within bounds with a 6h refresh cadence.

## Diagrams

### Component relationships

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Gateway Container (systemd)                                            │
│                                                                         │
│  ┌──────────────┐   RBUS IPC   ┌─────────────────┐                     │
│  │ rbus-elements│◀────────────▶│apparmor-simulator│                     │
│  │  (elements)  │              │  (C daemon)      │                     │
│  └──────────────┘              │  timerfd loop    │                     │
│                                │  RBUS method     │                     │
│        rbus.service            └────────┬─────────┘                    │
│            ▲                            │ libparodus                    │
│            │                            │ tcp://127.0.0.1:6666          │
│  ┌─────────┴──────┐                     ▼                               │
│  │ rbus (rtrouted)│            ┌──────────────────┐                     │
│  └────────────────┘            │    parodus        │                     │
│                                │  (WS client)      │                     │
└────────────────────────────────┤                   ├────────────────────┘
                                 └─────────┬─────────┘
                                           │ wss://talaria:6200
                                           │
┌──────────────────────────────────────────▼─────────────────────────────┐
│  XMiDT Stack (webpa network — event-sink runs as standalone compose)     │
│                                                                         │
│  ┌───────────────┐   WRP route   ┌──────────────┐                      │
│  │ talaria :6200 │──────────────▶│  caduceus    │                       │
│  └───────────────┘               │  (event fan- │                       │
│                                  │   out)        │                       │
│  ┌───────────────┐               └──────┬───────┘                      │
│  │  argus :6600  │◀─── read webhooks ───┘        │ HTTP POST           │
│  │ (webhook store│                                ▼                     │
│  └──────┬────────┘              ┌─────────────────────────┐            │
│                                 │  event-sink :8080        │            │
│         │ PUT /store/webhooks/… │  (Go service)            │            │
│         └───────────────────────│  - HMAC validation       │            │
│                                 │  - structured event log  │            │
│                                 │  - TTL refresh goroutine │            │
│                                 └──────────────────────────┘            │
└─────────────────────────────────────────────────────────────────────────┘
```

### Startup sequence

```
t=0   rbus starts
t=1   rbus-elements starts  →  registers static device data elements
t=2   parodus starts        →  connects to talaria, establishes WebSocket
t=3   apparmor-simulator starts
        ├── rbus_open()      →  self-registers Device.AppArmor.SimulateEvent() method handler
        ├── libparodus_init()→  registers "apparmor-simulator" with parodus
        └── timer_create()   →  arms 30s interval timer

t=4   event-sink starts (cloud)
        ├── PUT /store/webhooks/{id} to argus (retry loop)
        └── http.ListenAndServe(:8080)

t=5+  (every 30s)  apparmor-simulator emits WRP EVENT
        → parodus → talaria → caduceus
        → caduceus reads webhook from argus
        → caduceus POST to http://event-sink:8080/webhook
        → event-sink validates HMAC, logs event

t=6h  event-sink TTL refresh goroutine re-PUTs webhook to argus
```

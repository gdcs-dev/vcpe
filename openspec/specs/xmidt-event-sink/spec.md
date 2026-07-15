## Purpose
Receive and log WRP events from Caduceus via an Argus-backed webhook registration; a reusable, configurable sink for any XMiDT event type in the vCPE harness.

## Requirements

### Requirement: Argus-backed webhook registration on startup
The event-sink SHALL register its webhook with Argus on startup using the `ancla` library (`ancla.NewService` + `svc.Add` with an `ancla.InternalWebhook`), matching the ancla version the deployed Caduceus reads with (v0.3.12). It SHALL NOT hand-roll raw Argus PUT bodies, because the stored item format must exactly match what the deployed ancla can deserialize. The webhook registration SHALL include:
- `Config.URL`: `http://event-sink:8080/webhook`
- `Config.ContentType`: `application/json`
- `Config.Secret`: value of `WEBHOOK_SECRET` env var (required so Caduceus signs deliveries)
- `Events`: `[WEBHOOK_EVENTS_REGEX]` (default: `apparmor/.*`)
- `Matcher.DeviceID`: `[WEBHOOK_DEVICE_MATCHER]` (default: `.*`)
- `Duration` and `Until`: a validity window (12h duration; `Until` set well into the future)

If registration fails, the event-sink SHALL retry with exponential backoff (1s, 2s, 4s … max 30s) indefinitely until successful. The event-sink SHALL NOT begin logging events until initial registration succeeds.

#### Scenario: Successful registration on startup
- **WHEN** event-sink starts and Argus is reachable at `http://webpa:6600`
- **THEN** `svc.Add` succeeds
- **THEN** event-sink logs INFO "webhook registered" with events_regex and device_matcher values

#### Scenario: Argus unavailable at startup
- **WHEN** event-sink starts and Argus is not yet reachable
- **THEN** event-sink retries with exponential backoff
- **THEN** `/health` endpoint returns HTTP 200 during retry (liveness passes)
- **THEN** once Argus is reachable, registration succeeds and the TTL refresh loop starts

---

### Requirement: TTL refresh loop
The event-sink SHALL re-register the webhook via `svc.Add` every 6 hours to keep the Argus item alive. If a refresh fails, the event-sink SHALL log ERROR and retry at the next 6-hour interval without crashing.

#### Scenario: TTL refresh succeeds
- **WHEN** 6 hours have elapsed since last registration
- **THEN** event-sink re-registers the webhook via `svc.Add`
- **THEN** event-sink logs INFO "webhook TTL refreshed"

#### Scenario: TTL refresh fails
- **WHEN** the 6-hour refresh registration returns an error
- **THEN** event-sink logs ERROR
- **THEN** event-sink does not exit; retries at next 6-hour interval

---

### Requirement: HMAC signature validation
The event-sink HTTP webhook handler SHALL validate the `X-Webpa-Signature` header on every incoming POST before processing the payload. The header format is `sha1=<hmac-hex>` where the HMAC-SHA1 is computed over the raw request body using `WEBHOOK_SECRET` (Caduceus uses SHA1, per ancla's `DeliveryConfig.Secret`). Requests with missing, malformed, or invalid signatures SHALL be rejected with HTTP 401. The raw body SHALL NOT be logged if signature validation fails.

#### Scenario: Valid HMAC accepted
- **WHEN** Caduceus POSTs an event with a correct `sha1=` `X-Webpa-Signature`
- **THEN** event-sink returns HTTP 200 and logs the event

#### Scenario: Invalid HMAC rejected
- **WHEN** a POST arrives with an incorrect, non-`sha1=`, or missing `X-Webpa-Signature`
- **THEN** event-sink returns HTTP 401
- **THEN** event-sink logs WARNING with source IP (no payload logged)

---

### Requirement: WRP message decoding and structured logging
Caduceus delivers the WRP payload as the raw HTTP body and the WRP metadata (source, destination, content type) as `X-Xmidt-*`/`X-Webpa-*` headers. The event-sink SHALL reconstruct the WRP message from the request headers (via `wrphttp.SetMessageFromHeaders`) with the body as the payload, falling back to decoding the body as a msgpack/JSON WRP message when no header metadata is present. It SHALL log each validated event as a single-line JSON object to stdout using `log/slog`. The log entry SHALL include at minimum: `level`, `time`, `msg`, `dest`, `source`, `device_id` (extracted from WRP source), `content_type`, `payload_size`, and `payload`.

#### Scenario: Event received and logged
- **WHEN** event-sink receives a valid signed WRP EVENT POST from Caduceus
- **THEN** it emits one JSON log line to stdout containing dest, source, device_id, content_type, payload_size, and the decoded payload

---

### Requirement: Configurable webhook filter
The event-sink SHALL accept two environment variables to configure the Caduceus webhook filter at startup:
- `WEBHOOK_EVENTS_REGEX` (default: `apparmor/.*`): compiled as a regex; the event-sink SHALL fail fast with ERROR if the value does not compile as a valid Go regexp. The regex MUST NOT include the `event:` scheme prefix — Caduceus strips `event:` from the WRP destination before matching (a dest of `event:apparmor/denied/mac:...` is matched as `apparmor/denied/mac:...`).
- `WEBHOOK_DEVICE_MATCHER` (default: `.*`): same validation

The compiled regex values SHALL be logged at INFO on startup.

#### Scenario: Custom events regex
- **WHEN** `WEBHOOK_EVENTS_REGEX=device-status.*` is set
- **THEN** the Argus webhook item is registered with `Events: ["device-status.*"]`
- **THEN** event-sink receives only device-status events from Caduceus

#### Scenario: Invalid regex fails fast
- **WHEN** `WEBHOOK_EVENTS_REGEX=[invalid` is set
- **THEN** event-sink logs ERROR "invalid WEBHOOK_EVENTS_REGEX" and exits with non-zero status

#### Scenario: event: prefix is omitted
- **WHEN** the simulator emits dest `event:apparmor/denied/mac:aabbccddeeff` and `WEBHOOK_EVENTS_REGEX=apparmor/.*`
- **THEN** Caduceus matches the stripped destination `apparmor/denied/mac:aabbccddeeff` and delivers the event

---

### Requirement: Health endpoint
The event-sink SHALL expose `GET /health` on port 8080 returning HTTP 200 with body `{"status":"ok"}`. This endpoint SHALL respond immediately on startup, before Argus registration completes.

#### Scenario: Health check passes before registration
- **WHEN** event-sink has started its HTTP server but is still retrying Argus registration
- **THEN** `GET /health` returns HTTP 200

---

### Requirement: Deployment as a vCPE service type
The event-sink SHALL be deployable as a registered vCPE service type `event-sink`. The control plane SHALL render `services/event-sink/compose.env` (via the same `render.IfaceEnv` contract as webpa) during `vcpe apply`, and the container SHALL use the runtime-init init pattern (`runtime-init-event-sink` as ENTRYPOINT, `entrypoint.sh` as CMD). The container SHALL obtain its management IP via DHCP from BNG's dnsmasq so its hostname resolves for Caduceus delivery.

#### Scenario: Applied via manifest
- **WHEN** a manifest declares a service with `type: event-sink` and `vcpe apply` runs
- **THEN** the control plane renders `compose.env` and brings up the container
- **THEN** `vcpe down` tears the container down

#### Scenario: Hostname resolvable by Caduceus
- **WHEN** the event-sink container starts and acquires its IP via dhclient on eth0
- **THEN** BNG dnsmasq resolves `event-sink` to that IP
- **THEN** Caduceus delivers webhook POSTs to `http://event-sink:8080/webhook`

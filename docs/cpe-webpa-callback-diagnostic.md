# CPE to WebPA Callback Diagnostic

The `cpe-webpa-callback` journey proves that one supported Gateway or XB10 source
can emit a bounded marker event through the normal Parodus and WebPA pipeline,
and that `event-sink` receives the matching signed Caduceus callback.

It is not production-event tracing. It never injects an event directly into
Parodus or WebPA, accepts no caller-supplied credentials or target URLs, and
creates at most one event only after every prerequisite passes.

## Run the Journey

```bash
controlplane/bin/vcpe diagnose --name example --from gateway --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event apparmor/diagnostic \
  --device-id mac:02f9491df122

controlplane/bin/vcpe diagnose --name xb10 --from xb10 --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event devices/diagnostic \
  --device-id mac:001122334455
```

The source must be a Gateway or XB10 with the source-local AppArmor simulator.
The selected event and device ID must match the `event-sink` subscriber's
registration. `--allow-active-event` is explicit consent to generate the one
marker event.

A successful run has the following evidence chain:

```text
CPE simulator -> Parodus -> Talaria -> Caduceus -> event-sink
                              \-> Argus registration -/
```

The command first collects passive evidence from the selected CPE, WebPA, and
event-sink loopback diagnostic endpoints. It creates a fresh opaque,
lowercase, 64-hex correlation ID only after those checks pass. That ID joins the
single source event, Caduceus routing observation, and final callback receipt.

## Evidence Stages

### 1. Verify Selected Parodus Client

`[selected CPE application] --PASSED--> [Parodus]  verify selected Parodus client`

The selected CPE health service confirms `parodus.service` is active, then sends a
WRP Retrieve through Scytale for:

```text
mac:02f9491df122/parodus/service-status/apparmor-simulator
```

It validates the WRP transaction ID and response destination, then requires:

```json
{"service-status":"online"}
```

A pass proves that Parodus recognizes the selected `apparmor-simulator`
libparodus client as online. It is stronger than a process check alone.

### 2. Resolve Talaria

`[Parodus] --PASSED--> [Talaria]  resolve Talaria`

The selected CPE parses its configured Talaria devices endpoint, normally
`http://talaria:6200/api/v2/devices`, and resolves its hostname through the
source container's normal resolver. At least one address must be returned.

### 3. Connect to Talaria

`[Talaria] --PASSED--> [Talaria]  connect to Talaria`

The selected CPE opens and closes a TCP connection to the resolved Talaria host and
port, normally `6200`. This specifically validates network reachability after
DNS has succeeded and before HTTP authentication is attempted.

### 4. Authenticate to Talaria

`[Talaria] --PASSED--> [Talaria]  authenticate to Talaria`

The selected CPE sends an authenticated HTTP `GET` to Talaria's devices endpoint using
its locally configured credentials. A `401` or `403` is a confirmed
authentication failure. Any `2xx` response passes this stage.

### 5. Find Device Registration

`[Talaria] --PASSED--> [Device registration]  find device registration`

The selected CPE strictly decodes Talaria's bounded device-list response and
requires an exact match for the selected device ID. This confirms that the CPE
identity used for the source event is registered with Talaria.

### 6. Read Subscriber Intent

`[event-sink subscriber] --PASSED--> [Registration intent]  read subscriber intent`

The control plane reads the event-sink loopback diagnostic route:

```text
GET /diagnostics/webhook-subscriber/intent
```

The response describes event-sink's persisted callback contract: callback URL,
event filter, device matcher, content type, secret-configured state, and
registration timestamps. The response is schema- and bounds-validated. The
secret itself is never returned.

### 7. Reach Argus

`[Registration intent] --PASSED--> [Argus]  reach Argus`

WebPA healthd performs an authenticated lookup in Argus's authoritative
`webhooks` bucket using only WebPA-local endpoint configuration and
credentials. This pass proves WebPA can reach Argus.

### 8. Authenticate to Argus

`[Argus] --PASSED--> [Argus]  authenticate to Argus`

The same Argus lookup distinguishes rejected credentials from network failure.
A pass means Argus accepted WebPA's configured Basic authentication.

### 9. Find One Registration

`[Argus] --PASSED--> [Webhook registration]  find one registration`

WebPA decodes Argus's stored webhook registrations, normalizes callback URLs,
and requires exactly one registration matching event-sink's declared callback
URL. No match or multiple matches is a failure rather than a guess.

### 10. Verify Registration Freshness

`[Webhook registration] --PASSED--> [Webhook registration]  verify registration freshness`

The matching Argus registration must have a future expiry, a positive TTL when
provided, the expected 12-hour duration, and more than the six-hour refresh
window remaining. This prevents a near-expiry registration from being treated
as dependable delivery configuration.

### 11. Validate Event and Device Matcher

`[Webhook registration] --PASSED--> [Callback receipt]  validate event and device matcher`

WebPA compares the authoritative Argus registration against event-sink's
intent: callback URL, exactly one event filter, exactly one device matcher,
content type, and secret presence. The control plane also validates that the
chosen representative values match event-sink's declared patterns. In the
example:

- `apparmor/diagnostic` matches an event filter such as `apparmor/.*`.
- `mac:02f9491df122` matches the subscriber device matcher.

This prevents an active test that the real subscription would intentionally
ignore.

### 12. Accept One Marked Event

`[Parodus] --PASSED--> [Caduceus]  accept one marked event`

After all prerequisite stages pass, the control plane generates the correlation
ID and posts a bounded request to the selected CPE health daemon. The CPE writes
only the validated fields to its root-only Unix socket:

```text
/run/apparmor-simulator-diagnostic.sock
```

The AppArmor simulator requires the exact client name `apparmor-simulator`, a
valid event destination, the exact local device ID, and a lowercase 64-hex
correlation ID. It then calls `libparodus_send` for exactly one WRP event:

```text
source:      mac:02f9491df122/apparmor-simulator
destination: event:apparmor/diagnostic/mac:02f9491df122
payload:     cpe-webpa-callback marker and correlation ID
```

A pass means libparodus accepted the event locally. It is an emission
acknowledgement, not proof that a callback has been delivered yet.

### 13. Observe Routing Selection

`[Caduceus] --PASSED--> [Caduceus]  observe routing selection`

The control plane asks WebPA healthd for the Caduceus record associated with
the same correlation ID. WebPA healthd uses WebPA-local Caduceus credentials
to query its loopback Caduceus diagnostic endpoint. The returned routing record
must carry the expected correlation ID, a valid state, and an observation time.

This proves Caduceus selected and recorded a route for this specific marker
event.

### 14. Record Matching Callback Receipt

`[Caduceus] --PASSED--> [Callback receipt]  record matching callback receipt`

The control plane polls event-sink's bounded receipt route for the correlation
ID:

```text
GET /diagnostics/webhook-subscriber/receipts/<correlation-id>
```

Event-sink records a receipt only after it receives the callback and:

1. validates `X-Webpa-Signature` as an HMAC-SHA1 over the HTTP body using its
   local `WEBHOOK_SECRET`;
2. strictly decodes the marker payload and correlation ID;
3. confirms it is a `cpe-webpa-callback` marker; and
4. confirms Caduceus delivery headers are present.

The diagnostic requires the receipt source to be `caduceus`, not a direct test
callback. This is the terminal delivery proof: Caduceus delivered the signed
callback and event-sink accepted it.

## Result Semantics

`Result: passed` means all fourteen evidence stages passed for one correlation
ID. It proves a source-owned CPE event was accepted by Parodus, routed by the
deployed WebPA configuration, and accepted by event-sink as a signed
Caduceus callback.

A `failed` edge identifies a confirmed broken boundary. An `unknown` edge means
the diagnostic could not safely establish the required evidence. A missing
Caduceus routing record or event-sink receipt is therefore inconclusive, not
proof of successful delivery or rejection. Blocking failures cause downstream
stages to be `skipped` instead of speculatively evaluated.

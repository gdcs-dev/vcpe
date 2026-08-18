## Context

The event-sink already registers an `ancla.InternalWebhook` in Argus, refreshes its 12-hour duration every 6 hours, validates Caduceus HMAC-SHA1 callbacks, and reports initial registration through health. Those signals do not answer whether the authoritative stored item is fresh and conformant, whether Caduceus has selected it for a representative event, or where callback delivery failed.

The existing diagnostic framework provides a versioned graph, bounded loopback HTTP collection, capability discovery, safe evidence handling, deterministic ASCII/JSON rendering, and first-failure semantics. The CPE journey collects from one source endpoint. Webhook diagnosis requires two participants: the subscriber owns intended registration and callback receipt evidence, while WebPA owns Argus and Caduceus and has the correct network perspective for outbound callback delivery.

Argus's configured maximum item TTL is 24 hours. Event-sink writes a 12-hour webhook duration and refreshes every 6 hours. The ancla/chrysom client can retrieve items for an owner; inspection must use the deployed ancla model rather than hand-decoding a private storage representation.

## Verified Contract Spikes

The deployed `example` stack was exercised on 2026-08-14. These results are the
implementation contract for the active WebPA handler and are intentionally
bounded to one registered webhook and one marked synthetic event.

- `ancla.Service.GetAll(ctx)` is compatible with event-sink's empty-owner
  registration. It sends authenticated `GET /api/v1/store/webhooks` without the
  owner header and returns the bucket's `model.Item` values. The deployed Argus
  response contained the generated SHA-256 callback-URL ID, a decreasing item
  TTL, and deserializable `PartnerIDs`/`Webhook` data. The `Webhook` contains
  the URL, `application/json` content type, event and device matcher arrays,
  12-hour duration, and absolute `until`; the secret is present in the model but
  must be reduced to configured/equality state before any diagnostic response.
- Caduceus accepts `POST /api/v4/notify` with Basic authentication and a
  `application/msgpack` WRP simple-event message (`msg_type: 4`). A bounded
  marked event returned `202 Accepted` and was then asynchronously delivered to
  event-sink. The `202` proves queue acceptance only; receipt polling remains
  required for callback delivery. The ingress handler rejects unsupported media
  types with `415`, overload with `503`, and empty, malformed, non-simple-event,
  or invalid UTF-8 messages with `400`; authentication is enforced before the
  handler.
- The successful callback used Caduceus's normal `POST` delivery with the
  configured content type, WRP/X-WebPA headers, and `X-Webpa-Signature:
  sha1=<hmac>` over the exact transmitted body. Event-sink returned `200` after
  validating that signature. Until task 3.3 is implemented, a marked event is
  still logged and processed as a normal event; the diagnostic handler must
  change that result to an isolated `204` receipt.

## Goals / Non-Goals

**Goals:**

- Show the expected subscriber to Argus to Caduceus to callback path with first-failure attribution.
- Compare source-owned registration intent with authoritative Argus state without exposing the webhook secret.
- Test callback DNS, transport, HMAC validation, and HTTP acceptance from the WebPA network namespace.
- Test Caduceus registration selection and delivery using a representative synthetic WRP event.
- Reuse persisted loopback HTTP endpoints and the stable diagnostic graph/output contracts.
- Keep active effects explicit, bounded, correlated, and distinguishable from normal application events.

**Non-Goals:**

- Generating an event on a CPE or correlating a real CPE event end to end.
- Proving arbitrary application payload semantics after callback acceptance.
- Load, latency, retry, or soak testing.
- Supporting unmanaged external subscribers or arbitrary callback URLs.
- Parsing unbounded Argus, Caduceus, or subscriber logs.
- Adding a VS Code diagnostic overlay.

## Decisions

### 1. Add a webhook journey with explicit active consent

The command is:

```text
vcpe diagnose --name <deployment> --from <subscriber-service> --to webhook
  [--allow-active-callback --event <destination> --device-id <id>] [--json]
```

`--name`, `--from`, and `--to webhook` are required. Passive mode inspects intent and registration only. `--event` and `--device-id` are accepted only with `--allow-active-callback`; active mode requires both. The event is a normalized destination such as `apparmor/diagnostic`, and the device ID must be a valid WRP device identity.

Alternatives considered:

- Always send a callback: rejected because diagnosis would unexpectedly create subscriber traffic.
- Invent an event from the stored regex: rejected because general regular expressions cannot be safely or deterministically synthesized.

### 2. Resolve exactly one subscriber and one WebPA participant

The control plane resolves `--from` to a health-published subscriber instance and exactly one deployed `webpa` service. The subscriber type must register a webhook diagnostic provider; v1 supports `event-sink`. Multiple subscriber replicas require `--replica`, matching existing diagnostic selection behavior. Missing persisted endpoints fail before any active work.

The expected graph is provider-owned, but observations are merged from both participant endpoints. Neither endpoint receives container names or runtime details.

### 3. Split subscriber and WebPA diagnostic responsibilities

The subscriber endpoint advertises `webhook-subscriber` and returns bounded intent:

- callback URL without credentials or query secrets
- event regex and device matcher
- registration attempt/success/refresh timestamps and last bounded error category
- correlation observations for diagnostic callback receipts

The WebPA endpoint advertises `webhook-delivery` and owns:

- Argus reachability and authentication
- authoritative item lookup using deployed ancla/chrysom models
- registration freshness and conformance comparison
- direct signed callback probing from the WebPA namespace
- synthetic event injection through Caduceus

The control plane sends subscriber intent to WebPA only after validating and bounding it. It never transmits the webhook secret; WebPA obtains the stored secret from the matched Argus item and uses it only in memory.

Alternative considered: run everything from event-sink. Rejected because subscriber-local callback requests do not exercise Caduceus's DNS, route, or network namespace.

### 4. Match registrations by safe identity and fingerprint

Event-sink currently registers with an empty Argus owner and a generated item identity. Diagnosis retrieves the bounded webhook bucket view available to the deployed ancla/chrysom API and matches candidates by normalized callback URL plus event-sink provider identity. Zero matches fails `registration-present`; multiple matches fail as ambiguous rather than choosing one.

The matched item is compared against intended URL, event regex, device matcher, content type, and unexpired duration/until. Secret presence and equality are checked internally where possible but represented only as `configured` or `mismatch`; secret bytes, hashes, and lengths are not returned.

Freshness passes only when the authoritative item is unexpired and its remaining lifetime is compatible with the configured refresh policy. An item near expiry beyond the expected six-hour refresh cadence is reported stale even if technically not yet expired.

### 5. Use two active probes for causal attribution

Active mode first performs a direct signed diagnostic callback from the WebPA diagnostic handler to the stored callback URL. It uses a small reserved diagnostic body, a random correlation ID, the stored secret, strict DNS and HTTP deadlines, no redirects, and one attempt. This isolates:

1. callback DNS resolution
2. callback transport
3. signature validation
4. callback HTTP acceptance

The subscriber recognizes a reserved diagnostic marker and correlation ID after successful HMAC validation, records a bounded in-memory receipt, returns HTTP 204, and does not log or process it as a normal application event.

Only after the direct callback succeeds does WebPA inject a bounded synthetic WRP event into Caduceus's normal ingestion endpoint using the operator-supplied event and device identity plus a separate correlation marker. The subscriber receipt proves that Caduceus loaded a matching Argus registration, selected it, signed it, and delivered it. Failure after direct callback success is attributed to Caduceus selection/delivery, with filter or matcher mismatch highlighted when the supplied representative values do not match the stored registration.

Alternative considered: only inject through Caduceus. Rejected because a missing receipt cannot distinguish registration selection from callback DNS, transport, signature, or handler failures.

### 6. Extend the HTTP invocation without allowing arbitrary targets

Journey-specific invocations remain strict JSON. Subscriber requests contain correlation IDs only. WebPA requests contain bounded normalized subscriber intent, representative event/device inputs, and active-consent state. They cannot provide Argus credentials, webhook secrets, arbitrary callback URLs independent of the matched registration, executable commands, or Caduceus endpoint URLs.

All request and response bodies, candidate counts, evidence, polling attempts, timestamps, and messages are capped. Unknown fields and trailing JSON are rejected. `GET /health` remains unchanged and passive.

### 7. Poll bounded receipt state instead of parsing logs

After each active send, the control plane polls the subscriber diagnostic endpoint by correlation ID with a short overall deadline and bounded interval. Receipt records are memory-only, capped, expire automatically, and contain only correlation ID, source category (`direct` or `caduceus`), accepted timestamp, and HTTP outcome. A subscriber restart during diagnosis yields unknown receipt evidence rather than fabricated failure.

### 8. Preserve graph and exit semantics

The ordered v1 webhook stages are:

1. subscriber intent available
2. Argus reachable
3. Argus authentication accepted
4. registration present and unambiguous
5. registration fresh
6. registration conformant
7. direct callback DNS
8. direct callback transport
9. direct callback signature and HTTP acceptance
10. Caduceus event accepted
11. Caduceus registration selected and callback received

In passive mode, stages 7-11 are `unknown` with `active-callback-not-requested` and do not become confirmed failures. A healthy passive result is therefore valid but inconclusive and exits non-zero under the existing classification contract. Fully successful active mode exits zero.

## Risks / Trade-offs

- [Synthetic callbacks could be mistaken for real application events] -> Use a reserved marker, validate HMAC first, suppress normal processing/logging, and record only bounded receipt metadata.
- [Argus can contain duplicate matching registrations] -> Report ambiguity and do not send active callbacks.
- [Stored webhook secrets are highly sensitive] -> Keep them inside WebPA memory, never serialize them, and apply central redaction to all responses.
- [Representative event/device values may not match registration filters] -> Validate them against stored matcher configuration before Caduceus injection and report a filter/matcher failure.
- [Direct callback does not execute inside the Caduceus process] -> Treat it only as isolation of callback mechanics; require the second Caduceus ingestion/receipt probe for end-to-end delivery proof.
- [Caduceus ingestion may acknowledge before delivery] -> Poll subscriber receipt with a bounded deadline and distinguish ingestion acceptance from delivery receipt.
- [Subscriber or WebPA restarts erase diagnostic receipt state] -> Return unknown with timestamps rather than infer callback rejection.
- [Multi-participant observations are not atomic] -> Timestamp every observation and bound the whole journey duration.

## Migration Plan

1. Extend journey-specific diagnostic invocation and multi-participant orchestration behind existing interfaces.
2. Add event-sink intent and diagnostic-receipt endpoints without changing normal webhook behavior.
3. Add WebPA Argus inspection and direct callback isolation.
4. Add Caduceus synthetic event injection and receipt polling.
5. Expose CLI grammar, help, graph rendering, tests, and documentation.

The change is additive and requires no manifest or persisted-state migration. Rollback removes journey registration and diagnostic-only handlers; normal Argus registration and callback processing remain unchanged.

## Open Questions

None. The owner-empty Argus lookup and Caduceus ingestion contracts were
confirmed by the external contract spikes.

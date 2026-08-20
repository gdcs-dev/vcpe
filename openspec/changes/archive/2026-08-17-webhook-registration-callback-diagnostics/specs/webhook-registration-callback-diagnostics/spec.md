## ADDED Requirements

### Requirement: Deployment-targeted webhook diagnosis
The system SHALL provide `vcpe diagnose --name <deployment> --from <subscriber-service> --to webhook` for a deployed subscriber supported by the webhook diagnostic provider. The system SHALL resolve exactly one subscriber instance, exactly one WebPA service, and both persisted loopback diagnostic endpoints before active work.

#### Scenario: Event-sink subscriber is resolved
- **WHEN** an operator selects a single-replica event-sink and one WebPA service in a known deployment
- **THEN** the system diagnoses that subscriber's registration and callback path

#### Scenario: Unsupported subscriber is rejected
- **WHEN** the selected subscriber type has no webhook diagnostic provider
- **THEN** the command fails before participant HTTP requests with an actionable unsupported-type error

#### Scenario: Ambiguous WebPA participant is rejected
- **WHEN** the deployment contains more than one WebPA service
- **THEN** the command fails before active work and identifies the candidate services

### Requirement: Expected webhook diagnostic path
The diagnostic graph SHALL model subscriber intent, Argus reachability and authentication, authoritative registration presence, freshness and conformance, direct callback DNS and transport, signature and HTTP acceptance, Caduceus ingestion, and Caduceus callback receipt. The result SHALL preserve the common diagnostic edge states and first-failure semantics.

#### Scenario: Expected graph is available
- **WHEN** a valid webhook diagnosis starts
- **THEN** human and JSON output contain every expected registration and callback stage in deterministic order

#### Scenario: Passive graph retains callback stages
- **WHEN** active callback consent is absent
- **THEN** callback stages remain visible as not exercised rather than being omitted or inferred as passed

### Requirement: Subscriber-owned intent evidence
The subscriber diagnostic endpoint SHALL expose bounded intended callback URL, event filter, device matcher, content type, registration success/refresh timestamps, and a bounded registration error category. It MUST NOT return Argus credentials, webhook secrets, complete environment values, raw payloads, or logs.

#### Scenario: Registration intent is available
- **WHEN** event-sink has valid registration configuration
- **THEN** its diagnostic response contains normalized non-sensitive intent and the most recent registration timestamps

#### Scenario: Subscriber has not registered
- **WHEN** no initial Argus registration has succeeded
- **THEN** the subscriber-intent stage identifies the latest bounded registration error category without exposing the underlying credential or response body

### Requirement: Authoritative Argus registration diagnosis
The WebPA diagnostic endpoint SHALL query the deployed Argus `webhooks` bucket through compatible ancla/chrysom models, match the subscriber by normalized callback identity, and distinguish reachability, authentication, zero matches, multiple matches, freshness, and conformance. Stored secrets MUST remain internal and SHALL be represented only by non-sensitive configured/mismatch state.

#### Scenario: Matching fresh registration
- **WHEN** exactly one unexpired Argus item matches the intended callback URL, filter, matcher, and content type within the expected refresh policy
- **THEN** registration presence, freshness, and conformance stages pass

#### Scenario: Registration is missing
- **WHEN** no Argus item matches the subscriber callback identity
- **THEN** registration presence fails and dependent registration/callback stages are skipped

#### Scenario: Registration is ambiguous
- **WHEN** more than one Argus item matches the subscriber callback identity
- **THEN** registration presence fails as ambiguous and no active callback is sent

#### Scenario: Registration is stale
- **WHEN** the matching item is expired or its remaining lifetime is inconsistent with the six-hour refresh policy
- **THEN** registration freshness fails with expiration evidence that contains no secret data

#### Scenario: Registration fields differ
- **WHEN** the stored event filter, device matcher, callback URL, content type, or secret configuration differs from subscriber intent
- **THEN** registration conformance fails and identifies only the mismatched field names

### Requirement: Explicit active callback safety
The command SHALL NOT send a direct callback or synthetic Caduceus event unless `--allow-active-callback` is supplied. Active mode SHALL require `--event <destination>` and `--device-id <id>`. Those flags SHALL be rejected without active consent and SHALL be validated before participant requests.

#### Scenario: Passive diagnosis sends no callback
- **WHEN** the operator omits `--allow-active-callback`
- **THEN** registration stages run, no callback traffic is generated, and active stages report `active-callback-not-requested`

#### Scenario: Active inputs are required
- **WHEN** active consent is supplied without event or device identity
- **THEN** the command fails before participant requests with a required-input error

#### Scenario: Active-only inputs without consent are rejected
- **WHEN** event or device identity is supplied without active consent
- **THEN** the command fails before participant requests and explains the consent requirement

### Requirement: Direct signed callback isolation
After registration conformance passes in active mode, WebPA SHALL resolve the stored callback URL and send one bounded diagnostic callback from the WebPA network namespace using the stored secret and a random correlation ID. The system SHALL separately observe DNS, transport, signature validation, and callback HTTP acceptance. It MUST NOT follow redirects or return the secret, signature, callback body, or unrestricted response body.

#### Scenario: Direct callback succeeds
- **WHEN** DNS and transport succeed and the subscriber validates the signature and returns HTTP 204 for the diagnostic callback
- **THEN** direct callback DNS, transport, and acceptance stages pass and the subscriber records the direct correlation receipt

#### Scenario: Callback DNS fails
- **WHEN** the stored callback hostname does not resolve from WebPA
- **THEN** callback DNS fails and transport and acceptance are skipped

#### Scenario: Signature is rejected
- **WHEN** the subscriber rejects the signed diagnostic callback with HTTP 401
- **THEN** callback signature/acceptance fails without exposing the signature or secret

#### Scenario: Callback returns an application error
- **WHEN** the subscriber returns a non-success status other than authentication rejection
- **THEN** callback acceptance fails with only the bounded HTTP status as evidence

### Requirement: Caduceus routing and delivery diagnosis
After the direct callback succeeds, WebPA SHALL inject one bounded synthetic WRP event through Caduceus's normal ingestion path using the operator-supplied event and device identity plus a random diagnostic correlation marker. The subscriber SHALL acknowledge a correctly signed diagnostic callback without normal event processing. The control plane SHALL poll bounded subscriber receipt state to distinguish ingestion acceptance from callback receipt.

#### Scenario: Caduceus delivery succeeds
- **WHEN** Caduceus accepts the synthetic event, selects the stored registration, signs the callback, and the subscriber records its correlation ID
- **THEN** Caduceus ingestion and callback receipt stages pass

#### Scenario: Representative event misses filter
- **WHEN** the supplied event cannot match the stored event filter
- **THEN** diagnosis fails before injection with an event-filter mismatch

#### Scenario: Representative device misses matcher
- **WHEN** the supplied device identity cannot match the stored device matcher
- **THEN** diagnosis fails before injection with a device-matcher mismatch

#### Scenario: Ingestion accepted but callback absent
- **WHEN** Caduceus accepts the event but no subscriber receipt appears before the deadline
- **THEN** ingestion passes and Caduceus callback receipt is unknown or failed with a bounded delivery-timeout reason

### Requirement: Diagnostic callback isolation and receipt state
The subscriber SHALL recognize only a reserved, correctly signed diagnostic marker. It SHALL record bounded memory-only receipt metadata with automatic expiry, return HTTP 204, and SHALL NOT log or process the diagnostic payload as a normal application event. Invalid signatures SHALL continue to return HTTP 401 and SHALL NOT create receipt records.

#### Scenario: Marked callback is isolated
- **WHEN** a correctly signed callback contains the reserved diagnostic marker and correlation ID
- **THEN** the subscriber records only correlation ID, source category, timestamp, and outcome and does not emit the normal event log

#### Scenario: Receipt storage is bounded
- **WHEN** diagnostic callbacks exceed the configured count or age limit
- **THEN** the subscriber evicts old receipt metadata and retains no raw callback bodies

### Requirement: Multi-participant HTTP-only collection
The control plane SHALL collect webhook diagnostic evidence exclusively through the subscriber and WebPA persisted loopback HTTP endpoints. It SHALL use bounded timeouts, strict schemas, and bounded polling and SHALL NOT invoke Podman, Docker, Compose, container discovery, container exec, or unbounded logs.

#### Scenario: Diagnosis uses two persisted endpoints
- **WHEN** a webhook diagnosis runs
- **THEN** every observation is obtained from the resolved subscriber and WebPA loopback endpoints

#### Scenario: One participant is unavailable
- **WHEN** either participant endpoint cannot be reached
- **THEN** the command returns an actionable HTTP transport error and does not present a partial graph as complete

### Requirement: Webhook diagnostic evidence safety
Diagnostic requests and responses SHALL cap body sizes, candidate registrations, polling attempts, evidence, messages, and graph cardinality. They MUST NOT expose credentials, webhook secrets, HMAC values, stored item bodies, normal callback payloads, or unrestricted logs. Completed results SHALL pass central validation and redaction before human or JSON output.

#### Scenario: Secret-bearing evidence is redacted
- **WHEN** a participant returns a recognized credential, secret, signature, or authorization value
- **THEN** the control plane redacts it before emitting any output

#### Scenario: Excessive Argus candidates are refused
- **WHEN** Argus returns more candidate items than the diagnostic bound
- **THEN** WebPA rejects the response as ambiguous or excessive without serializing the item bodies

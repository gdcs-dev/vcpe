## ADDED Requirements

### Requirement: Correlated CPE-to-callback event diagnosis
The system SHALL provide a `cpe-webpa-callback` diagnostic journey that establishes whether one bounded, reserved diagnostic event travelled from a supported CPE application's selected receive-enabled Parodus client through WebPA routing to a selected registered subscriber callback. The completed result SHALL contain an ordered graph spanning CPE application evidence, Talaria connectivity and registration, subscriber registration validity, CPE event acceptance, WebPA/Caduceus routing, and a matching subscriber receipt.

#### Scenario: Fully correlated event succeeds
- **WHEN** all selected CPE, WebPA, routing, and subscriber checks pass and the subscriber records the matching callback receipt
- **THEN** the journey reports every required edge as passed, emits no `firstFailure`, and exits zero

#### Scenario: Partial paths do not prove delivery
- **WHEN** the CPE event is accepted or routing acknowledges it but no matching subscriber receipt is established before the bounded deadline
- **THEN** the journey does not report end-to-end delivery as passed

### Requirement: Explicit bounded active-event safety
The journey SHALL require explicit active-event consent and bounded validated selections for the CPE client service, subscriber service, representative event destination, and device identity before accessing deployment state or invoking diagnostic endpoints. It SHALL emit at most one reserved marked event per invocation and MUST NOT accept arbitrary WRP bodies, credentials, callback URLs, executable commands, or target endpoints.

#### Scenario: Active consent is required
- **WHEN** an operator omits the active-event consent flag
- **THEN** the command rejects the invocation before any diagnostic HTTP request or event generation

#### Scenario: Unmatched event selection is rejected
- **WHEN** the selected event destination or device identity does not satisfy the subscriber's authoritative registration matcher
- **THEN** the journey fails before sending the marked event and identifies the matcher boundary

### Requirement: End-to-end correlation and callback isolation
The CPE, WebPA, and subscriber diagnostic participants SHALL use one opaque bounded correlation identity for a reserved diagnostic marker. The subscriber SHALL record a receipt only after normal callback authenticity validation, return an isolated diagnostic response, and MUST NOT log or process that marked callback as a normal application event. The control plane SHALL poll only the selected subscriber's persisted loopback diagnostic endpoint for the bounded receipt state.

#### Scenario: Correctly signed marked callback is isolated
- **WHEN** the subscriber receives a valid signed callback containing the current reserved diagnostic marker
- **THEN** it records the matching receipt, returns the diagnostic response, and bypasses normal event handling

#### Scenario: Invalid signature is not accepted as a receipt
- **WHEN** a marked callback fails normal signature validation
- **THEN** the subscriber does not record a successful receipt and preserves its existing invalid-callback behavior

### Requirement: Causal multi-participant result semantics
The journey SHALL collect CPE, WebPA, and subscriber observations exclusively through persisted loopback HTTP endpoints, merge them into one centrally validated and redacted diagnostic graph, and identify the earliest confirmed failed edge as `firstFailure`. Unknown or restart-lost observations SHALL be inconclusive and SHALL NOT be presented as confirmed delivery failures. The control plane SHALL NOT invoke a container runtime, Compose, container CLI, or container exec.

#### Scenario: CPE-path failure prevents event generation
- **WHEN** application, Talaria, or device-registration prerequisites fail
- **THEN** the event-generation and downstream callback edges are skipped and no marked event is sent

#### Scenario: Subscriber restart makes receipt inconclusive
- **WHEN** the subscriber endpoint loses its in-memory receipt state during the bounded polling window
- **THEN** the receipt edge is unknown with a stable reason and the command exits non-zero without claiming callback rejection

### Requirement: Correlation evidence safety and output
Human and JSON output SHALL use the existing diagnostic result schema, safety limits, and central redaction. Output SHALL identify selected services and stable stage, reason, and remediation IDs, but MUST NOT expose complete correlation IDs, event bodies, WRP credentials, Argus credentials, webhook secrets, signatures, authorization headers, or unrestricted participant environment data.

#### Scenario: JSON identifies the callback boundary without secrets
- **WHEN** routing succeeds but callback delivery fails
- **THEN** JSON identifies the callback edge and stable remediation without serializing secret or raw-event material

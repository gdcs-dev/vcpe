## MODIFIED Requirements

### Requirement: Correlated CPE-to-callback event diagnosis
The system SHALL provide a `cpe-webpa-callback` diagnostic journey that establishes whether one bounded, reserved diagnostic event travelled from a supported Gateway or XB10 CPE application's selected receive-enabled Parodus client through WebPA routing to a selected registered subscriber callback. The completed result SHALL contain an ordered graph spanning CPE application evidence, Talaria connectivity and registration, subscriber registration validity, CPE event acceptance, WebPA/Caduceus routing, and a matching subscriber receipt. Gateway and XB10 source support SHALL be enabled only when the source workload explicitly exposes the fixed source-local bounded event interface; other CPE sources SHALL remain unsupported.

#### Scenario: Fully correlated event succeeds
- **WHEN** all selected CPE, WebPA, routing, and subscriber checks pass and the subscriber records the matching callback receipt
- **THEN** the journey reports every required edge as passed, emits no `firstFailure`, and exits zero

#### Scenario: Partial paths do not prove delivery
- **WHEN** the CPE event is accepted or routing acknowledges it but no matching subscriber receipt is established before the bounded deadline
- **THEN** the journey does not report end-to-end delivery as passed

#### Scenario: XB10 source emits a bounded correlated event
- **WHEN** an operator selects an XB10 source that advertises `cpe-webpa-callback` and supplies all valid active-event inputs with explicit consent
- **THEN** the journey uses XB10's source-local fixed event interface to emit at most one marked event and evaluates the same ordered graph and callback receipt semantics as Gateway

#### Scenario: Unsupported CPE source is rejected
- **WHEN** an operator selects a CPE source type other than Gateway or XB10 for `--to callback`
- **THEN** the control plane rejects provider selection without invoking an active event interface
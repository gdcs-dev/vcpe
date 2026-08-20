## ADDED Requirements

### Requirement: Deployment-targeted Talaria device inventory
The system SHALL provide `vcpe diagnose --name <deployment> --from <webpa-service> --to devices` for a deployed WebPA source. It SHALL resolve exactly one selected WebPA instance and its persisted loopback diagnostic endpoint without resolving a CPE source, subscriber, Parodus participant, Argus participant, or Caduceus participant. The journey SHALL be passive and SHALL use the selected WebPA instance's configured Talaria endpoint as the authority for currently connected device sessions.

#### Scenario: Inventory succeeds for the selected WebPA source
- **WHEN** an operator selects a supported single-replica WebPA service in a known deployment
- **THEN** the control plane invokes that source's persisted loopback endpoint to inventory Talaria's current device registry

#### Scenario: Multiple WebPA replicas require selection
- **WHEN** the selected WebPA service has more than one replica and the operator omits `--replica`
- **THEN** the command fails before diagnostic HTTP activity and requires an explicit replica index

#### Scenario: Unsupported source is rejected
- **WHEN** the selected source type does not provide the Talaria device inventory journey
- **THEN** the command fails before participant HTTP requests with an actionable unsupported-type error

### Requirement: Bounded operator-visible Talaria session inventory
The WebPA diagnostic endpoint SHALL read Talaria's authenticated device list and SHALL return every validated current device session when the registry contains at most 64 entries. Each inventory entry SHALL include the raw Talaria device ID, pending queue depth, bytes sent, messages sent, bytes received, messages received, duplications, connection time, and uptime. The endpoint MUST NOT return Talaria credentials, authorization headers, request or response payloads, WebSocket details, device metadata, arbitrary Talaria fields, or container-runtime details.

#### Scenario: Connected sessions are returned
- **WHEN** Talaria contains validated connected sessions
- **THEN** the inventory returns the selected operator-visible fields for every session

#### Scenario: Empty registry is valid
- **WHEN** Talaria contains no connected device sessions
- **THEN** the journey passes and returns an empty structured device list

#### Scenario: Registry exceeds the diagnostic bound
- **WHEN** Talaria returns more than 64 connected device sessions
- **THEN** the journey reports an incomplete or unknown bounded result and omits device entries rather than returning a partial list

#### Scenario: Invalid Talaria session is withheld
- **WHEN** Talaria returns a device session with an invalid identifier, counter, connection time, uptime, or bounded field
- **THEN** the endpoint does not serialize the raw response or invalid value and reports a bounded invalid-inventory observation

### Requirement: Deterministic Talaria device inventory output
The system SHALL sort validated device sessions by raw device ID before returning them. It SHALL expose the list as structured `talariaDevices` data in JSON and SHALL render the selected WebPA source, Talaria target, operator-visible device fields, and zero-entry state in human output. Existing diagnostic journeys SHALL omit device inventory fields and retain their current output contracts.

#### Scenario: JSON output remains structured
- **WHEN** an operator runs `vcpe diagnose --name <deployment> --from <webpa-service> --to devices --json`
- **THEN** the output contains the `talaria-devices` journey and structured `talariaDevices` entries without packing sessions into evidence strings

#### Scenario: Human output is deterministic
- **WHEN** an operator runs a successful inventory without `--json`
- **THEN** the output lists device sessions in device-ID order and identifies an empty inventory when no sessions exist

### Requirement: Passive Talaria inventory safety
The Talaria device inventory journey SHALL perform no device-directed request, device connection, Talaria control operation, callback, Caduceus event injection, registration mutation, webhook mutation, or arbitrary Talaria query. It SHALL use bounded loopback HTTP collection, strict result validation, and central defensive copying. It SHALL reject active-event, client-service, subscriber, and subscriber-replica options before state access or network activity.

#### Scenario: Inventory does not generate device traffic
- **WHEN** an operator runs a valid `--to devices` inventory request
- **THEN** the system only reads Talaria's current device registry through WebPA and generates no device or registration-changing traffic

#### Scenario: Cross-journey option is rejected
- **WHEN** an operator supplies `--subscriber`, `--client-service`, `--allow-active-callback`, `--allow-active-event`, `--event`, `--device-id`, or `--subscriber-replica` with `--to devices`
- **THEN** the command fails before deployment resolution or diagnostic HTTP activity
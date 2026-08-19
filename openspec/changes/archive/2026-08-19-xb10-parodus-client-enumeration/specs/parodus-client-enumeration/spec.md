## MODIFIED Requirements

### Requirement: Deployment-targeted Parodus client enumeration
The system SHALL provide `vcpe diagnose --name <deployment> --from <service> --to parodus` for a deployed Gateway or XB10 source instance. The command SHALL resolve the selected source and its persisted loopback diagnostic endpoint from deployment state without requiring a WebPA service. When the source has multiple replicas, the command SHALL require `--replica <index>`; otherwise it SHALL select replica zero. The command SHALL reject `--client-service`, subscriber, callback, and active-event options before state access or network activity.

#### Scenario: Gateway enumeration succeeds
- **WHEN** an operator selects a supported single-replica Gateway source for `--to parodus`
- **THEN** the system invokes the source-local Parodus enumeration journey through its persisted loopback endpoint

#### Scenario: XB10 enumeration succeeds
- **WHEN** an operator selects a supported single-replica XB10 source for `--to parodus`
- **THEN** the system invokes the selected XB10 instance's source-local Parodus enumeration journey through its persisted loopback endpoint

#### Scenario: Multiple XB10 replicas require selection
- **WHEN** the selected XB10 service has more than one replica and the operator omits `--replica`
- **THEN** the command fails before diagnostic HTTP activity and requires an explicit replica index

#### Scenario: WebPA is not required
- **WHEN** the selected deployment has no WebPA service
- **THEN** a valid Parodus enumeration request still resolves and executes against its Gateway or XB10 source

#### Scenario: Incompatible client selection is rejected
- **WHEN** an operator supplies `--client-service` with `--to parodus`
- **THEN** the command fails before deployment resolution or an active diagnostic request

### Requirement: Bounded source-owned client-list retrieval
The selected Gateway or XB10 diagnostic endpoint SHALL query `<device>/parodus/client-list` through its configured Scytale endpoint using a correlated WRP Retrieve. It SHALL validate the HTTP response, MessagePack WRP envelope, transaction correlation, destination, and JSON payload before returning a result. A valid payload SHALL contain at most 64 lexicographically sorted client-service identifiers and a boolean `truncated` value. The endpoint SHALL expose the validated names as `parodusClients` and truncation state as `parodusClientsTruncated`; it SHALL NOT expose credentials, raw WRP envelopes, arbitrary response data, or container-runtime details.

#### Scenario: Valid client list is returned
- **WHEN** Parodus returns a correlated payload with a valid sorted `client-list` and `truncated` value
- **THEN** the journey reports a passed Parodus client-list observation and returns those values in its structured diagnostic result

#### Scenario: XB10 uses source-owned identity and configuration
- **WHEN** an XB10 source runs the client-list journey
- **THEN** its health daemon derives the device identity and uses its configured Scytale endpoint and credentials without accepting those values from the control-plane request

#### Scenario: Empty client list is valid
- **WHEN** Parodus returns `{"client-list":[],"truncated":false}`
- **THEN** the journey reports a passed observation with an empty structured client list

#### Scenario: Invalid list is inconclusive
- **WHEN** the client-list payload is missing, malformed, over the limit, unsorted, or contains an invalid client-service identifier
- **THEN** the journey reports an unknown observation with bounded remediation and omits the client-list result fields

#### Scenario: Patched response is truncated
- **WHEN** Parodus returns a valid client list with `truncated` set to true
- **THEN** the journey returns the bounded listed names and reports `parodusClientsTruncated` as true
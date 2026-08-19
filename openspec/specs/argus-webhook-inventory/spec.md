## Purpose
Define the deployment-targeted, passive Argus webhook inventory diagnostic journey.

## Requirements

### Requirement: Deployment-targeted Argus webhook inventory
The system SHALL provide `vcpe diagnose --name <deployment> --from <webpa-service> --to webhooks` for a deployed WebPA source. It SHALL resolve exactly one selected WebPA instance and its persisted loopback diagnostic endpoint without resolving a subscriber, CPE source, Parodus participant, or Caduceus participant. The journey SHALL be passive and SHALL use the selected WebPA instance's configured Argus endpoint as the authoritative registration source.

#### Scenario: Inventory succeeds for the selected WebPA source
- **WHEN** an operator selects a supported single-replica WebPA service in a known deployment
- **THEN** the control plane invokes that source's persisted loopback endpoint to inventory its Argus `webhooks` bucket

#### Scenario: Multiple WebPA replicas require selection
- **WHEN** the selected WebPA service has more than one replica and the operator omits `--replica`
- **THEN** the command fails before diagnostic HTTP activity and requires an explicit replica index

#### Scenario: Unsupported source is rejected
- **WHEN** the selected source type does not provide the Argus webhook inventory journey
- **THEN** the command fails before participant HTTP requests with an actionable unsupported-type error

### Requirement: Authoritative bounded Argus registration inventory
The WebPA diagnostic endpoint SHALL query the deployed Argus `webhooks` bucket through compatible ancla/chrysom models and SHALL inventory every safely decoded registration returned by that bucket, regardless of which service or external tool created it. It SHALL return at most 64 registrations. A valid inventory entry SHALL include only an opaque fingerprint, normalized callback URL, event filters, device matchers, content type, expiry time, optional TTL seconds, and secret-presence state. The endpoint MUST NOT return secrets, Argus credentials, authorization headers, raw stored item bodies, raw callback URLs, callback query values, URL userinfo, signatures, payloads, or container-runtime details.

#### Scenario: Registrations from multiple creators are returned
- **WHEN** Argus contains registrations created by event-sink and an independent registrant
- **THEN** the inventory returns both safely decoded registrations without filtering by creator or manifest ownership

#### Scenario: Empty bucket is a valid inventory
- **WHEN** the authoritative Argus `webhooks` bucket contains no registrations
- **THEN** the journey passes and returns an empty structured registration list

#### Scenario: Inventory has more than the safe bound
- **WHEN** Argus returns more than 64 registrations
- **THEN** the journey reports an incomplete or unknown bounded result and omits registration entries rather than returning a partial list

#### Scenario: Unsafe registration data is withheld
- **WHEN** an Argus item has an invalid callback identity, invalid content type, malformed time or TTL value, or unsafe bounded field
- **THEN** the endpoint does not serialize the raw item or unsafe value and reports a bounded invalid-inventory observation

### Requirement: Deterministic safe inventory output
The system SHALL sort validated registration entries by opaque fingerprint before returning them. It SHALL expose the list as structured `webhookRegistrations` data in JSON and SHALL render the selected WebPA source, Argus target, safe registration fields, and zero-entry state in human output. Existing diagnostic journeys SHALL omit inventory fields and retain their current output contracts.

#### Scenario: JSON output remains structured
- **WHEN** an operator runs `vcpe diagnose --name <deployment> --from <webpa-service> --to webhooks --json`
- **THEN** the output contains the `argus-webhooks` journey and structured `webhookRegistrations` entries without packing registrations into evidence strings

#### Scenario: Human output is deterministic
- **WHEN** an operator runs a successful inventory without `--json`
- **THEN** the output lists safe registration entries in fingerprint order and identifies an empty inventory when no registrations exist

### Requirement: Passive inventory safety
The Argus webhook inventory journey SHALL perform no callback, Caduceus event injection, registration refresh, registration mutation, deletion, or arbitrary Argus query. It SHALL use bounded loopback HTTP collection, strict result validation, and central redaction. It SHALL reject active-event, client-service, subscriber, and subscriber-replica options before state access or network activity.

#### Scenario: Inventory does not generate traffic or mutate registrations
- **WHEN** an operator runs a valid `--to webhooks` inventory request
- **THEN** the system only reads the authoritative Argus bucket through WebPA and generates no callback or registration-changing traffic

#### Scenario: Cross-journey option is rejected
- **WHEN** an operator supplies `--subscriber`, `--client-service`, `--allow-active-callback`, `--allow-active-event`, `--event`, `--device-id`, or `--subscriber-replica` with `--to webhooks`
- **THEN** the command fails before deployment resolution or diagnostic HTTP activity
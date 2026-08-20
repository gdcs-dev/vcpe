## Purpose
Define bounded, deployment-targeted diagnostics for the CPE-to-WebPA connectivity path.

## Requirements

### Requirement: Deployment-targeted CPE-to-WebPA diagnosis
The system SHALL provide `vcpe diagnose --name <deployment> --from <service> --to webpa --client-service <name>` for a deployed source instance supported by the CPE-to-WebPA diagnostic provider. The command SHALL require a valid receive-enabled libparodus service name and SHALL resolve its source and WebPA target from persisted deployment state before executing active probes. When the source service has multiple replicas, the system SHALL require `--replica <index>`; when it has one replica, the system SHALL select replica zero if the flag is omitted. The source image SHALL NOT infer or configure a default client service.

#### Scenario: Single-replica source is selected
- **WHEN** an operator diagnoses a supported single-replica Gateway or XB10 service in a known deployment
- **THEN** the system selects replica zero and diagnoses its configured path to the deployment's WebPA service

#### Scenario: Multi-replica source requires selection
- **WHEN** an operator diagnoses a source service with multiple replicas without `--replica`
- **THEN** the command fails before active probing and identifies the valid replica indexes

#### Scenario: Unsupported source is rejected
- **WHEN** an operator selects a source type with no CPE-to-WebPA diagnostic provider
- **THEN** the command fails before active probing with an error identifying the unsupported type

#### Scenario: Ambiguous WebPA target is rejected
- **WHEN** the deployment contains more than one service of type `webpa`
- **THEN** the command fails before active probing and identifies the ambiguous candidate services

#### Scenario: Client service override is selected
- **WHEN** an operator supplies `--client-service config`
- **THEN** the source queries the fixed Parodus service-status destination for `config`

#### Scenario: Client service is required
- **WHEN** an operator omits `--client-service`
- **THEN** the command fails before deployment resolution or active probing with a required-flag error

#### Scenario: Invalid client service is rejected
- **WHEN** an operator supplies a client service containing a slash, whitespace, or more than the bounded identifier length
- **THEN** the command fails before active probing without constructing or sending a WRP request

### Requirement: Expected CPE-to-WebPA diagnostic path
The diagnostic result SHALL model an ordered expected path containing application-to-Parodus, Talaria DNS resolution, Talaria transport reachability, Talaria authentication, and authoritative device registration stages. Expected metadata SHALL identify the deployment, source service and replica, target service, sanitized endpoints, and derived device identity when available without exposing secrets.

#### Scenario: Expected path is shown before observations
- **WHEN** a valid CPE-to-WebPA diagnosis begins
- **THEN** the result graph identifies every expected stage from the CPE application boundary through Talaria device registration

#### Scenario: Expected metadata excludes secrets
- **WHEN** the source uses credentials to query Talaria
- **THEN** neither human nor JSON output contains the credential, authorization header, token, or complete source environment

### Requirement: Ordered bounded diagnostic observations
The source diagnostic endpoint SHALL execute trusted source-owned probes in prerequisite order with per-probe and overall deadlines. The system SHALL distinguish application-to-Parodus evidence, DNS resolution, transport reachability, authentication, and registration rather than deriving those stages from unbounded log parsing. For a configured receive-enabled Parodus client, the source SHALL query `<device>/parodus/service-status/<service>` through Scytale and validate the correlated WRP response. Diagnostic requests and user-controlled manifest values MUST NOT supply executable command text, credentials, arbitrary target URLs, WRP destinations, or probe definitions.

#### Scenario: Healthy path evaluates every stage
- **WHEN** every prerequisite probe passes within its deadline
- **THEN** the result contains a passed observation for every expected path edge

#### Scenario: DNS fails before transport
- **WHEN** Talaria name resolution fails in the source workload
- **THEN** the DNS edge is failed and transport, authentication, and registration edges are skipped

#### Scenario: Authentication is distinguished from reachability
- **WHEN** the source resolves and reaches Talaria but Talaria rejects its credentials
- **THEN** resolution and transport are passed, authentication is failed, and registration is skipped

#### Scenario: Probe exceeds its deadline
- **WHEN** a diagnostic probe does not complete within its configured deadline
- **THEN** its edge is unknown with a timeout reason and dependent edges are skipped

### Requirement: HTTP-only diagnostic transport
Each diagnostic-capable source instance SHALL serve diagnostic routes on the same container-local HTTP port and persisted loopback endpoint used for its common health endpoint. `GET /diagnostics` SHALL return bounded capability metadata without running active probes. `POST /diagnostics/cpe-webpa` SHALL require a bounded strict JSON invocation containing exactly one `clientService`, run the bounded CPE-to-WebPA journey in the source workload's network namespace, and return a versioned diagnostic response. The source SHALL reject empty or missing client services, unknown fields, oversized bodies, trailing JSON values, and invalid client identifiers before executing the journey. The control plane SHALL NOT invoke Podman, Docker, Compose, a container CLI, or container exec to collect diagnostic observations.

#### Scenario: Capability discovery is passive
- **WHEN** `vcpe` requests `GET /diagnostics` from a source instance
- **THEN** the endpoint identifies whether `cpe-webpa` is supported without executing its active probes

#### Scenario: Active diagnosis uses persisted loopback HTTP
- **WHEN** an operator diagnoses a supported source instance
- **THEN** `vcpe` sends `POST /diagnostics/cpe-webpa` to that instance's persisted loopback endpoint and does not discover or execute its container

#### Scenario: Health behavior remains passive
- **WHEN** `vcpe status` requests `GET /health` from a diagnostic-capable instance
- **THEN** no active diagnostic journey runs and the existing common health response contract is unchanged

#### Scenario: Unsupported journey is reported
- **WHEN** capability discovery does not advertise `cpe-webpa`
- **THEN** the command fails before sending the active diagnostic request with an actionable unsupported-journey error

### Requirement: Causal graph state semantics
Every diagnostic edge SHALL have exactly one state from `passed`, `failed`, `unknown`, or `skipped` and SHALL declare whether an unresolved observation blocks following stages. The result SHALL identify the earliest failed edge as `firstFailure`. Unknown evidence SHALL make the result inconclusive but SHALL NOT be labeled a confirmed causal failure. A downstream edge whose blocking prerequisite failed or is unknown SHALL be skipped rather than failed.

#### Scenario: First confirmed failure is emphasized
- **WHEN** transport reachability is the earliest probe that positively fails
- **THEN** the transport edge is `firstFailure` and no downstream skipped edge is presented as an additional failure

#### Scenario: Application client evidence is unavailable
- **WHEN** the source provider cannot obtain a valid correlated Parodus client-status response
- **THEN** the application-to-Parodus edge is unknown with bounded explanatory evidence rather than passed, and independently verifiable Parodus-to-WebPA stages continue

#### Scenario: Application client is online
- **WHEN** Scytale returns a correlated Parodus client-status response whose bounded payload is `{"service-status":"online"}`
- **THEN** the application-to-Parodus edge passes and identifies the configured client service without exposing credentials or raw WRP metadata

#### Scenario: Application client is offline
- **WHEN** Scytale returns a correlated Parodus client-status response whose bounded payload is `{"service-status":"offline"}`
- **THEN** the application-to-Parodus edge fails with an actionable libparodus-registration reason while independently verifiable downstream stages continue

#### Scenario: Unknown blocking prerequisite has no first failure
- **WHEN** a blocking prerequisite probe is unknown and no earlier probe failed
- **THEN** dependent edges are skipped and the result has no `firstFailure`

### Requirement: Visual human diagnostic output
Without `--json`, the command SHALL render deterministic ASCII showing expected nodes, state-marked edges, skipped downstream stages, and the first confirmed failure or inconclusive boundary. The output SHALL include bounded evidence and actionable remediation for the selected failed or unknown boundary and SHALL remain understandable without terminal color.

#### Scenario: Human output highlights DNS failure
- **WHEN** Talaria DNS resolution fails
- **THEN** the ASCII path marks application and Parodus context, marks the DNS boundary failed, marks downstream stages skipped, and displays DNS-specific remediation

#### Scenario: Human output is usable without color
- **WHEN** output is redirected to a file or displayed in a terminal without color support
- **THEN** textual state labels and ASCII connectors preserve all diagnostic meaning

### Requirement: Stable machine-readable diagnostic graph
With `--json`, the command SHALL emit a document with schema version `vcpe.dev/diagnostic/v1`, journey identity, source and target identities, ordered nodes, ordered edges, observations, optional first-failure edge ID, and stable reason and remediation IDs. JSON output SHALL be generated from the same validated result model as human output.

#### Scenario: Healthy JSON graph
- **WHEN** a diagnosis completes with every edge passed and `--json` is requested
- **THEN** the JSON document contains all expected nodes and passed edges and omits `firstFailure`

#### Scenario: Failed JSON graph identifies cause
- **WHEN** device registration is the first confirmed failed stage and `--json` is requested
- **THEN** `firstFailure` references the registration edge and that edge includes stable registration-failure and remediation identifiers

### Requirement: Diagnostic evidence safety and limits
The system SHALL cap HTTP request and response bodies, probe duration, evidence count, graph cardinality, and diagnostic message length. The system SHALL centrally validate and redact the completed result before human rendering or JSON serialization. Diagnostic output MUST NOT contain raw logs, request payloads, credentials, tokens, authorization headers, or unrestricted environment data.

#### Scenario: Oversized diagnostic response is rejected
- **WHEN** a diagnostic endpoint returns an HTTP body larger than the configured response limit
- **THEN** the system rejects it as an invalid protocol response without buffering unbounded content

#### Scenario: Provider returns sensitive evidence
- **WHEN** provider output contains a recognized credential or authorization value
- **THEN** the central safety pass redacts the value before any output is emitted

### Requirement: Diagnostic result exit behavior
The command SHALL exit zero only when every required diagnostic edge passes. It SHALL emit a valid result and exit non-zero when a stage fails or the journey is inconclusive. Invocation, topology-resolution, and HTTP transport or protocol errors that prevent construction of a valid result SHALL exit non-zero with actionable error text rather than a partial graph presented as complete.

#### Scenario: Fully connected path succeeds
- **WHEN** all required stages pass
- **THEN** the command emits the completed graph and exits zero

#### Scenario: Confirmed failure returns diagnostic result
- **WHEN** a stage positively fails
- **THEN** the command emits the completed graph with `firstFailure` and exits non-zero

#### Scenario: Diagnostic endpoint unavailable before observations
- **WHEN** the source's persisted loopback endpoint cannot be reached and no valid diagnostic result can be constructed
- **THEN** the command exits non-zero with an actionable HTTP transport error

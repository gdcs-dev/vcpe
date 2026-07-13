## MODIFIED Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly. The phases SHALL include service type planning, host-network preflight, image lifecycle decisions, typed rendering, compose group application, health verification, status inspection, and generated artifact state recording.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

#### Scenario: Apply uses type-driven phases
- **WHEN** an operator applies a deployment through `vcpe up`
- **THEN** the plan includes service type ordering, required image actions, render artifacts, compose groups, host-network intents, and health checks before runtime mutation begins

### Requirement: Typed service catalog planning
The system SHALL plan service lifecycle actions from the service type registry and the manifest, deriving service ordering, image policy, render contracts, compose groups, and health checks without per-customer catalog tables. Cross-service ordering across separate compose projects SHALL follow manifest `dependsOn`, applied in dependency order on startup and reverse dependency order on teardown.

#### Scenario: Service ordering is deterministic
- **WHEN** a deployment enables multiple services with `dependsOn` relationships
- **THEN** the planner emits deterministic start, health, stop, and rollback order across compose projects without relying on bash command order

#### Scenario: Cross-project dependsOn governs lifecycle order
- **WHEN** services in separate compose projects declare `dependsOn`
- **THEN** the planner starts dependencies first and tears them down in reverse, independent of any intra-project compose `depends_on`

## ADDED Requirements

### Requirement: Schema-versioned state cutover
The system SHALL stamp the persisted state root with the `vcpe.dev/v1` schema version and MUST refuse to reconcile against a non-empty state root whose stamp is missing or mismatched, requiring an explicit operator-initiated state reset before applying v1 manifests.

#### Scenario: Mismatched state blocks reconcile
- **WHEN** the persisted state root holds data with a missing or non-`vcpe.dev/v1` schema stamp
- **THEN** apply fails before mutation with an actionable error directing the operator to reset state

#### Scenario: First v1 apply has no prior snapshot
- **WHEN** the operator applies a v1 manifest against a freshly reset, schema-stamped state root
- **THEN** the apply proceeds as a clean create without flagging a disruptive teardown

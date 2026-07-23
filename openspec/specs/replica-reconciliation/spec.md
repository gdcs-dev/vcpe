## Purpose
Define named replica indexing, delta-only apply semantics, and persisted replica state for multi-replica service reconciliation.

## Requirements

### Requirement: Stable indexed replica naming
The system SHALL name compose services for every replica using a 1-based index suffix (`{service}-{n}`) regardless of the total replica count. A service named `client` with `replicas: 1` SHALL produce compose service `client-1`; with `replicas: 2` it SHALL produce `client-1` and `client-2`.

#### Scenario: Single-replica service uses indexed name
- **WHEN** a manifest declares `replicas: 1` for service `client`
- **THEN** the compose project contains exactly one service named `client-1` and its container is named `{project}_client-1_1`

#### Scenario: Multi-replica service uses 1-based indexed names
- **WHEN** a manifest declares `replicas: 3` for service `client`
- **THEN** the compose project contains services `client-1`, `client-2`, and `client-3`

#### Scenario: Scaling up preserves existing replica name
- **WHEN** service `client` is running with `replicas: 1` (container `{project}_client-1_1`) and the operator increases to `replicas: 2`
- **THEN** container `{project}_client-1_1` continues running unchanged and a new container `{project}_client-2_1` is created

### Requirement: Delta-only apply on replica count change
The system SHALL compute the difference between the desired replica count and the previously applied replica count stored in deployment state, and SHALL only create or remove the delta containers. Replicas within the desired count that are already running MUST NOT be restarted or recreated.

#### Scenario: Scale up adds only new replicas
- **WHEN** `vcpe up` is run with `replicas: 2` on a deployment that was previously applied with `replicas: 1`
- **THEN** exactly one new container is started (index 2) and the existing container (index 1) is left running unchanged

#### Scenario: Scale down removes only excess replicas
- **WHEN** `vcpe up` is run with `replicas: 1` on a deployment that was previously applied with `replicas: 3`
- **THEN** containers for indices 2 and 3 are stopped and removed; the container for index 1 continues running unchanged

#### Scenario: Unchanged replica count produces no container mutations
- **WHEN** `vcpe up` is run with the same `replicas` value as the previously applied manifest
- **THEN** no containers for that service are started, stopped, or recreated

#### Scenario: Scale down removes excess replicas in reverse index order
- **WHEN** a service is scaled from `replicas: 3` to `replicas: 1`
- **THEN** the system removes index-3 container first, then index-2 container, leaving index-1 running

### Requirement: Persisted replica count tracks applied state
The system SHALL persist the replica count applied for each service in the deployment state record. This persisted count SHALL serve as the baseline for the next delta computation and SHALL be updated on every successful apply.

#### Scenario: Replica count is written on successful apply
- **WHEN** `vcpe up` successfully applies a service with `replicas: 2`
- **THEN** the deployment state for that service records `replicaCount: 2`

#### Scenario: Replica count baseline is used on next apply
- **WHEN** `vcpe up` runs against a deployment with persisted `replicaCount: 1` and the manifest now declares `replicas: 2`
- **THEN** the planner identifies index 1 as existing and index 2 as new, and only starts the index-2 container

### Requirement: vcpe down tears down all live replicas
The system SHALL tear down all replica containers for a service based on the persisted replica count, regardless of the replica count declared in the current manifest file.

#### Scenario: Down removes all persisted replicas
- **WHEN** `vcpe down` is run on a deployment with persisted `replicaCount: 3` for service `client`
- **THEN** all three replica containers (`client-1`, `client-2`, `client-3`) are stopped and removed

## ADDED Requirements

### Requirement: Go-owned host-network mutation controller
The system SHALL perform bridge, NAT, firewall, and attachment mutations through Go-owned host-network controller phases derived from typed topology intents and persisted state. Host-network mutation MUST NOT be delegated to behavior-owning shell scripts.

#### Scenario: Host-network intents are reconciled by Go controller
- **WHEN** apply includes host-network intents
- **THEN** the Go host-network controller reconciles required bridge, NAT, and firewall state and records phase outcomes

#### Scenario: Shell mutation ownership is removed
- **WHEN** host-network mutation is required for deployment convergence
- **THEN** no legacy shell script owns the mutation flow

### Requirement: Host capability preflight and delegation support
The system SHALL preflight required host-network capabilities before mutation and SHALL support macOS through Podman-machine or delegated Linux host execution model.

#### Scenario: Missing capabilities fail before mutation
- **WHEN** required host-network capabilities are unavailable
- **THEN** apply fails before runtime mutation with actionable diagnostics

#### Scenario: macOS delegated host path is supported
- **WHEN** the operator runs on macOS with supported Podman-machine delegation
- **THEN** host-network reconciliation uses the delegated model and reports capability/preflight outcomes consistently
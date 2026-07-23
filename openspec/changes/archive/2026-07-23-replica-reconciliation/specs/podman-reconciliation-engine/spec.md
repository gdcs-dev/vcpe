## MODIFIED Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly. The phases SHALL include service type planning, host-network preflight, image lifecycle decisions, typed rendering, replica delta computation, compose group application, health verification, status inspection, and generated artifact state recording.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

#### Scenario: Apply uses type-driven phases
- **WHEN** an operator applies a deployment through `vcpe up`
- **THEN** the plan includes service type ordering, required image actions, render artifacts, replica delta computation, compose groups, host-network intents, and health checks before runtime mutation begins

#### Scenario: Re-apply with increased replica count is convergent
- **WHEN** `vcpe up` is run with `replicas: 2` on a service that was previously applied with `replicas: 1`
- **THEN** exactly one new container is created and the original container is left running, producing a total of two running containers for that service

#### Scenario: Re-apply with decreased replica count is convergent
- **WHEN** `vcpe up` is run with `replicas: 1` on a service that was previously applied with `replicas: 2`
- **THEN** the excess container is removed and one container remains running, producing a total of one running container for that service

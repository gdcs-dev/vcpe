## ADDED Requirements

### Requirement: vcpe manifest build wizard
The system SHALL provide a `vcpe manifest build` subcommand that runs an interactive four-phase wizard (identity, networks, services, output) to produce a complete and valid `vcpe.dev/v1` manifest. Each prompt SHALL display a default value in brackets; pressing Enter without input SHALL accept the default. When stdin is not a TTY the wizard SHALL return all defaults without blocking. When `--manifest <path>` is supplied the wizard SHALL load the existing manifest and pre-fill all prompts with its current values, writing the result to `--output <path>` (default: `<stem>-updated.yaml`).

#### Scenario: Wizard produces valid manifest
- **WHEN** an operator completes `vcpe manifest build` interactively
- **THEN** the output file passes `vcpe plan --manifest <output>` without validation errors

#### Scenario: Wizard is non-blocking in CI
- **WHEN** `vcpe manifest build` is run with stdin redirected (not a TTY)
- **THEN** all prompts return their default values and the manifest is written without user interaction

#### Scenario: Update mode pre-fills from existing manifest
- **WHEN** an operator runs `vcpe manifest build --manifest existing.yaml`
- **THEN** every prompt shows the current value from the existing manifest as its default

#### Scenario: macvlan network presents interface menu
- **WHEN** an operator selects `driver: macvlan` for a network during the wizard
- **THEN** the wizard fetches available interfaces from the Podman host, displays them with type/state/address details, and prompts for a selection by number or name

#### Scenario: Service config pre-filled from networks
- **WHEN** an operator configures a `bng` service attached to a `wan` network
- **THEN** the DHCP4 subnet, pool ranges, and routers option are pre-filled from the wan network's CIDR, pool, and gateway respectively

#### Scenario: Interface discovery failure falls back to manual entry
- **WHEN** the wizard cannot discover interfaces (no `ip` command, no Podman machine)
- **THEN** the wizard falls back to a free-text prompt for the parent interface name and continues normally

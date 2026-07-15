## MODIFIED Requirements

### Requirement: Operator documentation reflects current vcpe CLI surface
The project READMEs SHALL accurately reflect the current vcpe command surface, registered service types, manifest discovery workflow, Homebrew default channel, and release workflow. Specifically: `README.md` SHALL document the `manifest list`, `manifest build`, `build`, `push`, `release`, and `version` commands; service types documented SHALL include `event-sink`, `xb10`, `oktopus`, and `generic-container` in addition to `bng`, `gateway`, and `webpa`; `packaging/homebrew/README.md` SHALL document `release` as the default Homebrew channel and `vcpe up` as the primary deployment command; `services/event-sink/README.md` SHALL document `vcpe up --manifest` as the primary deployment path with standalone `docker compose` as a secondary development-only path.

#### Scenario: Root README covers all top-level commands
- **WHEN** a developer reads `README.md`
- **THEN** every top-level vcpe command (`init`, `build`, `push`, `release`, `up`, `plan`, `down`, `list`, `manifest`, `status`, `logs`, `config`, `state`, `version`) is described or referenced

#### Scenario: Homebrew README shows correct default channel
- **WHEN** a developer reads `packaging/homebrew/README.md`
- **THEN** the channels table and update example both show `release` as the default channel

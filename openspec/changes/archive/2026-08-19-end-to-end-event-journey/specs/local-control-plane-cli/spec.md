## ADDED Requirements

### Requirement: Active callback journey CLI invocation
The `vcpe diagnose` command SHALL support `--to callback` for the `cpe-webpa-callback` journey. This invocation SHALL require `--name`, `--from`, `--client-service`, `--subscriber`, `--event`, `--device-id`, and `--allow-active-event`; it SHALL accept `--replica`, `--subscriber-replica`, and `--json` where valid. The command SHALL reject missing, malformed, cross-journey, or incompatible flags before state access, diagnostic HTTP requests, or active event generation.

#### Scenario: Valid callback journey is dispatched
- **WHEN** an operator supplies all required callback journey flags for a supported deployment and source
- **THEN** the Go operator dispatches the bounded active journey through persisted loopback diagnostic endpoints

#### Scenario: Callback journey rejects unsafe incomplete invocation
- **WHEN** an operator omits active-event consent or a required client, subscriber, event, or device selection
- **THEN** the command exits non-zero with an actionable flag error before performing active work

#### Scenario: Help distinguishes active traffic
- **WHEN** an operator runs `vcpe diagnose --help`
- **THEN** the structured help identifies callback-journey required flags and states that it generates one bounded diagnostic event only after explicit consent
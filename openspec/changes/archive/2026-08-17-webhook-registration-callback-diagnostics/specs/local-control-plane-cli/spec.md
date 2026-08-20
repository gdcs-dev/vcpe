## ADDED Requirements

### Requirement: Webhook diagnostic CLI journey
The Go-owned `vcpe diagnose` command SHALL support `--to webhook` with required `--name <deployment>` and `--from <subscriber-service>`. It SHALL support passive registration diagnosis by default and active callback diagnosis only with `--allow-active-callback --event <destination> --device-id <id>`. It SHALL support the existing human and `--json` diagnostic graph outputs and deployment selection semantics.

#### Scenario: Passive webhook diagnosis parses
- **WHEN** an operator runs `vcpe diagnose --name example --from event-sink --to webhook`
- **THEN** the command selects the passive webhook journey without requiring CPE-specific `--client-service`

#### Scenario: Active webhook diagnosis parses
- **WHEN** an operator additionally supplies `--allow-active-callback --event apparmor/diagnostic --device-id mac:001122334455`
- **THEN** the command validates and carries those active inputs to the webhook diagnostic journey

#### Scenario: CPE-specific flag is rejected
- **WHEN** an operator supplies `--client-service` with `--to webhook`
- **THEN** the command fails before state access and identifies the flag as valid only for `--to webpa`

#### Scenario: Webhook-specific flag is rejected for WebPA journey
- **WHEN** an operator supplies `--allow-active-callback`, `--event`, or `--device-id` with `--to webpa`
- **THEN** the command fails before state access and identifies the incompatible journey

#### Scenario: Webhook help documents safety
- **WHEN** an operator runs `vcpe diagnose --help`
- **THEN** help text documents passive webhook diagnosis, explicit active consent, required active inputs, examples, and the fact that active mode generates diagnostic callback traffic

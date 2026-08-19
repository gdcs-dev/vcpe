## 1. Source-Local Enumeration

- [x] 1.1 Add the bounded `parodus-clients` journey, structured client-list result fields, and validation for sorted client-service identifiers.
- [x] 1.2 Implement correlated Scytale WRP retrieval of `parodus/client-list` with bounded unknown-result handling and unit tests.

## 2. Diagnostic Integration

- [x] 2.1 Register the Gateway-only source-local provider and resolve `--to parodus` without a WebPA target or endpoint.
- [x] 2.2 Wire the health daemon capability and active route, then render structured JSON and deterministic human client-list output.

## 3. CLI And Verification

- [x] 3.1 Add `--to parodus` grammar, cross-journey option rejection, help text, and focused CLI tests.
- [x] 3.2 Run focused diagnostic and app tests, build the CLI and health daemon, validate the OpenSpec change, and verify the live Gateway command.
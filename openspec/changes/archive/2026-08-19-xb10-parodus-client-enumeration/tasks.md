## 1. Diagnostic Provider and Resolution

- [x] 1.1 Refactor the `parodus-clients` diagnostic provider to accept a supported source type and derive its expected source graph node from the selected service.
- [x] 1.2 Register the existing Parodus enumeration journey for XB10 while retaining Gateway registration and unsupported-source rejection.
- [x] 1.3 Add provider and resolver tests for XB10 source selection, persisted loopback resolution, and explicit replica selection when XB10 has multiple replicas.

## 2. XB10 Capability Wiring

- [x] 2.1 Advertise `--diagnostic=parodus-clients` in the XB10 `vcpe-healthd` systemd service without adding callback capability or event-injection configuration.
- [x] 2.2 Add health-daemon coverage that verifies XB10 enables the existing Parodus client-list journey and retains source-local invocation validation.

## 3. Diagnostic Results and Regression Coverage

- [x] 3.1 Add orchestration coverage for a successful XB10 Parodus client list, including the source-neutral graph and existing structured client/truncation fields.
- [x] 3.2 Retain or extend Gateway regression coverage to prove provider refactoring preserves its source graph and output contract.
- [x] 3.3 Verify shared CPE/WebPA protocol tests still enforce source-owned identity, correlated Scytale WRP retrieval, 64-client bound, sorting, truncation, and sanitized unknown failures.

## 4. Documentation and Validation

- [x] 4.1 Update operator-facing diagnostic documentation to list XB10 as a supported `--to parodus` source and clarify that it enumerates receive-enabled registered clients only, not active callback delivery.
- [x] 4.2 Run `gofmt` on changed Go files and focused Go tests for diagnostic, type registration, and health-daemon packages.
- [x] 4.3 Stage the XB10-target health daemon, rebuild/reconcile an XB10 deployment with the patched Parodus route, and verify `vcpe diag --name <deployment> --from <xb10-service> --to parodus --json` returns the bounded structured result.
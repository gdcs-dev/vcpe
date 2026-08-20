## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| Extend Parodus enumeration sources to XB10 | `controlplane/internal/diagnostic/provider.go`, `controlplane/internal/types/types.go`, `services/xb10/container/vcpe-healthd.service` | No |

---

## Decisions

### Decision: Supported source types for Parodus enumeration
[BREAKING]
Recommendation: Extend the existing `parodus-clients` journey to XB10 where the same patched route is available.
Decision: Support both Gateway and XB10 sources for `vcpe diag --from <service> --to parodus`.
Rationale: XB10 and Gateway have compatible source-local identity, Scytale, and Parodus client-list protocol behavior; separate journeys would duplicate an identical operator contract.

Q: How should XB10 expose its patched Parodus client list?
A: Through the same diagnostic journey and `--to parodus` target used by Gateway.

---

### Decision: Callback-diagnostic scope
Recommendation: Treat active callback delivery as a separate capability from Parodus enumeration.
Decision: Do not add XB10 active callback diagnostics in this change.
Rationale: Gateway's active callback journey depends on an AppArmor simulator event-injection socket that XB10 does not provide.

Q: Does "same level" include Gateway's active callback diagnostic path?
A: No; this change is limited to parity for the patched Parodus client-list route.

---

### Decision: Collection ownership and payload contract
Recommendation: Reuse the existing XB10-local health-daemon handler and bounded response validation unchanged.
Decision: Keep Scytale endpoint selection, credentials, device identity derivation, WRP correlation, 64-client limit, sort validation, and truncation semantics source-owned and unchanged.
Rationale: The existing probe already matches XB10's readiness identity/configuration pattern and avoids exposing transport details to the control plane.

Q: Should the control plane directly query Scytale or receive a new raw XB10 response shape?
A: No; reuse the existing bounded source-local collection contract.
## Decisions

### Decision: Device inventory source and scope
[BREAKING]
Recommendation: Add a dedicated passive WebPA-local journey backed by Talaria's connected-device registry.
Decision: Proceed with the recommended approach using `vcpe diag --from <webpa-service> --to devices`.
Rationale: Talaria is the authoritative local source for current WebSocket sessions and already supplies the device-registration evidence used by CPE diagnosis.

Q: Which WebPA API should supply the new diagnostic inventory?
A: Talaria `GET /api/v2/devices` should supply the inventory through WebPA.

---

### Decision: Operator-visible device data
[BREAKING]
Recommendation: Prefer opaque device identifiers unless an operator need requires raw identities.
Decision: Expose raw device IDs, pending queue depth, message/byte counters, connection time, and uptime in the operator-visible inventory.
Rationale: The operator explicitly approved full inventory visibility.

Q: May device identities and session details appear in the operator-visible inventory?
A: Everything can appear in an operator-visible inventory.

---

### Decision: Passive bounded result semantics
[BREAKING]
Recommendation: Cap and validate the translated inventory independently of Talaria's in-process cap, returning no partial records when that cap is exceeded.
Decision: Proceed with the recommended bounded all-or-incomplete behavior.
Rationale: A partial list cannot be represented as an authoritative inventory, and the diagnostic contract needs a stable resource bound.

Q: How should an unexpectedly large Talaria registry be represented?
A: Use a bounded diagnostic result rather than serializing an unbounded or partial list.
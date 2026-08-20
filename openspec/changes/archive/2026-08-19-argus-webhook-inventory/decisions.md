## Decisions

### Decision: Inventory ownership and scope
[BREAKING]
Recommendation: Use WebPA-local Argus access to list every safely decoded registration in the authoritative `webhooks` bucket.
Decision: Proceed with the recommendation.
Rationale: The user requires registrations from anywhere, rather than only event-sink registrations or manifest-defined services.

Q: Where should the list come from and which registrations should it include?
A: The list must come from Argus so it includes registrations from anywhere.

---

### Decision: Diagnostic target shape
[BREAKING]
Recommendation: Add a distinct `--to webhooks` target with WebPA as `--from`.
Decision: Proceed with the recommendation.
Rationale: Argus inventory is passive and source-owned by WebPA, while `--to webhook` validates one subscriber's intent, freshness, conformance, and optionally delivery.

Q: Should inventory be folded into the existing subscriber-focused webhook diagnostic?
A: No. It is a separate Argus-wide inventory.

---

### Decision: Safe result fields and bounds
[BREAKING]
Recommendation: Return a deterministic, bounded structured inventory containing only fingerprint, normalized callback identity, filters, matchers, content type, expiry or TTL state, and secret-presence state.
Decision: Proceed with the recommendation.
Rationale: The command must make all authoritative registrations visible without creating an unbounded or secret-bearing Argus inspection API.

Q: What information should the inventory expose?
A: Registered webhooks from Argus, subject to the existing diagnostic safety model.
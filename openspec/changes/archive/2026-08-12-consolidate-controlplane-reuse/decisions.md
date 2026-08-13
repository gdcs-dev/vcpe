## Decisions

### Decision: Source of truth for the proposal
Recommendation: Evaluate the current `sol-consolidate` control-plane tree and ratified specifications independently.
Decision: Proceed with the current tree as the sole implementation baseline and do not rely on other consolidation implementations.
Rationale: The proposal should solve the observed duplication and contracts on their own merits rather than inherit another branch's assumptions.

Q: Should prior implementations influence this proposal?
A: No. Forget about the other implementations and create the most comprehensive proposal possible.

---

### Decision: Proposal completeness
Recommendation: Produce the full proposal, architecture, delta specification, design, and implementation task set needed to make the change apply-ready.
Decision: Proceed with the comprehensive artifact set, including explicit compatibility constraints, non-goals, migration order, and verification gates.
Rationale: The consolidation crosses renderer, registry, application, image backend, and utility ownership boundaries and requires more detail than a minimal refactoring note.

Q: How comprehensive should the change proposal be?
A: Create the most comprehensive proposal possible.

---
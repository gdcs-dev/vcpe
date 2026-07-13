---
description: Exhaustive pre-implementation interview - eliminate all ambiguities before coding
---

Conduct a systematic one-question-at-a-time interview to eliminate all implementation ambiguities. Produce `decisions.md`.

**Input**: The argument after `/opsx-revise` is a change name or goal (e.g., `/opsx-revise add-payments-flow`). If omitted, the agent will ask.

---

## Steps

### 1. Set up the change

```bash
openspec new change "<name>"
```

**Same-session continuation**: If an architect session just concluded in this context window, use that context directly — skip re-reading what is already known.

### 2. Read context (cold start)

Read `architecture.md` if present, plus relevant codebase sections. Don't bias questions toward existing patterns — ask what's architecturally correct.

### 3. Conduct the interview

Ask ONE question at a time with a recommended answer. Wait for response before continuing.

**Challenge bad answers**: State the violated principle, the concrete consequence, give an alternative, then wait for decision.

**Breaking vs. non-breaking**: Present both paths when existing code is affected.

### 4. Domain checklist coverage

After AI-derived questions are exhausted, check 11 domains: data model, API contracts, auth/authz, error handling, state management, testing, security, performance, observability, deployment, edge cases.

Skip irrelevant domains. Continue for uncovered relevant ones.

### 5. Simulated-implementer subagent check

"Running an implementation coverage check..."

Subagent: imagines implementing the change with only the decisions and codebase. Reports blocking questions or SATISFIED. Resume interview if gaps found.

### 6. Write decisions.md

```bash
openspec instructions decisions --change "<name>" --json
```

Structure: BREAKING CHANGES summary table (if any), then individual decision entries with `[BREAKING]` and `[OVERRIDE]` markers.

### 7. Align architecture.md

If `architecture.md` exists in the change directory: run an alignment subagent to find places where `decisions.md` CONTRADICTS (not merely refines) content in `architecture.md`. Refinements that add detail on topics `architecture.md` was silent about are not contradictions — do not flag them.

- If the subagent returns `NO_CONTRADICTIONS`: tell the user "architecture.md is consistent with all decisions — no updates needed."
- If contradictions are found: show a summary of affected sections, ask the user to confirm, then patch only the contradicted sections.
- If `architecture.md` does not exist: skip silently.

---

## Guardrails

- One question at a time — core discipline of revise mode
- Always include recommendation — user should never have to guess
- Challenge, then accept — raise concerns, user's decision is final
- Record overrides truthfully — decisions.md is a complete record

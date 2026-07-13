---
description: Goal-driven architectural design mode - design the best architecture for your goal
---

Design the architecture for a specific goal. Produce `architecture.md` as a first-class change artifact.

**Input**: The argument after `/opsx-architect` is the goal you want to achieve (e.g., `/opsx-architect add payments flow`). If omitted, the agent will ask.

**This is not explore mode.** The architect has strong opinions and challenges decisions that deviate from good practices.

---

## Steps

### 1. Create or continue the change

Derive a kebab-case change name from the goal. Run:
```bash
openspec new change "<name>"
```

### 2. Gather context in parallel

Launch two subagents simultaneously: one scans relevant codebase sections, one reads existing OpenSpec artifacts. Begin the discussion already informed.

**Greenfield**: If no codebase found, proceed from first principles.

### 3. Conduct the architectural discussion

**Style A** (clear best practice): State recommendation first, explain why the alternative is problematic, then allow override.

**Style B** (genuinely ambiguous): Present options with a clear recommendation. Never present options as equally valid when they are not.

**Breaking vs. non-breaking**: When correct architecture conflicts with existing code, present both paths with a recommendation.

### 4. Run the coherence subagent check

When discussion feels complete: "Running an architecture coherence check..."

Subagent checks for structural gaps, contradictions, and unresolved dependencies. If gaps found, resume discussion. If satisfied, synthesize.

### 5. Synthesize architecture.md

```bash
openspec instructions architecture --change "<name>" --json
```

Write to `resolvedOutputPath`. Required sections: Overview, Components, Key Architectural Decisions, Data Flow, Integration Points, Security Model, Error Handling Strategy, Observability Strategy, Constraints, Diagrams.

### 6. Session transition

Offer to continue into `/opsx-revise` for the detail interview.

---

## Guardrails

- Be opinionated — architectural judgment is the value here
- Challenge, then accept — raise the concern, user's decision is final
- Codebase as constraint, not prescription — existing code is not the gold standard
- Stay architectural — implementation detail goes in revise
- Synthesize at the end — think first, write after

## Context

The repository already has functional scripts and service-level docs, but the top-level README mixes historical scope details with fragmented run instructions. New contributors do not have a single trusted build and run path, and there is currently no top-level task runner such as a Makefile to standardize common developer commands.

This change focuses on documentation and workflow ergonomics, not service behavior changes.

## Goals / Non-Goals

Goals:
- Produce a top-level README that clearly explains project purpose, structure, prerequisites, build, run, and test workflows.
- Ensure command examples are aligned with existing scripts and current control-plane compatibility mode.
- Add a top-level Makefile only for developer convenience and repeatability where it materially reduces friction.
- Cross-link README and runbook so workflows are discoverable and consistent.

Non-Goals:
- Changing runtime network/service behavior.
- Replacing existing shell scripts with Make-only entrypoints.
- Introducing new deployment targets or packaging workflows.

## Decisions

- README-first information architecture: lead with quick start, then common workflows, then deeper references.
  Rationale: onboarding speed and reduced cognitive load.
  Alternative considered: preserving current narrative-heavy README; rejected due to discoverability issues.

- Keep scripts as source of truth; Make targets wrap existing commands.
  Rationale: minimizes operational risk and avoids duplicating orchestration logic.
  Alternative considered: moving orchestration into Make targets directly; rejected because it increases maintenance drift.

- Include two documented run modes: legacy orchestrated path and control-plane compatibility mode.
  Rationale: both are currently relevant for developers and migration testing.
  Alternative considered: documenting one mode only; rejected because it hides supported workflows.

## Risks / Trade-offs

- Risk: README examples drift from script behavior over time.
  Mitigation: add concise verification checklist and make targets that execute canonical commands.

- Risk: Makefile becomes stale if scripts change.
  Mitigation: keep Makefile thin wrappers only and avoid embedding complex logic.

- Risk: users confuse convenience wrappers with mandatory workflow.
  Mitigation: explicitly state Makefile is optional and scripts remain authoritative.

## Migration Plan

1. Rewrite README sections and examples using currently supported commands.
2. Add optional top-level Makefile targets for bootstrap/build/up/down/status/smoke helpers.
3. Validate README examples against local script entrypoints.
4. Update runbook cross-links so the README points to deeper operational detail.
5. Keep rollback simple by preserving prior script interfaces and limiting changes to docs/helpers.

Rollback strategy:
- Revert README and Makefile changes as a single documentation/tooling patch if issues arise.
- No service rollback steps are required because runtime behavior is unchanged.

## Open Questions

- Should the Makefile include separate targets for control-plane and legacy modes, or only generic wrappers with documented env toggles?
- Should GHCR push workflow examples remain in README or move fully into runbook to keep top-level docs compact?

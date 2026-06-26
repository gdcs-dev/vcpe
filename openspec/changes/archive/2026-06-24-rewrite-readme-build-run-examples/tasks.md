## 1. Discovery And Validation

- [x] 1.1 Audit current README commands against existing scripts and runbook references
- [x] 1.2 Define final README section structure for overview, prerequisites, quick start, workflows, and references
- [x] 1.3 Identify which repeated command flows should be exposed as optional Make targets

## 2. README Rewrite

- [x] 2.1 Rewrite top-level README introduction and project structure overview
- [x] 2.2 Add verified build and run examples for legacy orchestration mode
- [x] 2.3 Add verified build and run examples for control-plane compatibility mode
- [x] 2.4 Add troubleshooting and rollback pointers with links to deeper docs

## 3. Optional Build Helpers

- [x] 3.1 Add top-level Makefile with thin wrapper targets for bootstrap, build, up, down, status, and smoke checks
- [x] 3.2 Ensure Make targets call existing scripts without introducing behavior drift
- [x] 3.3 Document Makefile usage in README as optional convenience commands

## 4. Consistency And Verification

- [x] 4.1 Cross-check README examples against scripts under scripts and smoke tests under tests/smoke
- [x] 4.2 Verify documentation links to docs/runbook and other referenced files are accurate
- [x] 4.3 Run markdown and shell syntax checks for any newly added examples/helpers

## 5. Final Review

- [x] 5.1 Review readability and onboarding flow from a first-time contributor perspective
- [x] 5.2 Summarize changes and capture any follow-up documentation debt

## Why

The top-level project documentation is partially outdated and does not provide a clean, task-oriented path for new contributors to build, run, and troubleshoot the project. A rewritten README with verified examples and optional build helpers is needed to reduce setup friction and improve developer onboarding.

## What Changes

- Rewrite the top-level README to present project purpose, architecture overview, prerequisites, build steps, run flows, and common examples.
- Add a clear quick-start path for both legacy shell orchestration and control-plane compatibility mode.
- Add documented command examples for profile management, smoke tests, and rollback paths.
- Introduce optional developer build helpers through a top-level Makefile when it improves repeatability.
- Ensure README command examples align with existing scripts and runbook guidance.

## Capabilities

### New Capabilities
- developer-readme-and-build-workflow: Provides a canonical top-level developer workflow including build, run, test, and troubleshooting guidance with optional Make targets.

### Modified Capabilities
- None.

## Impact

- Documentation surfaces: README and runbook cross-references.
- Developer experience tooling: optional top-level Makefile and helper targets.
- Operator/developer workflows: standardized examples for scripts under scripts and tests under tests/smoke.
- No runtime API or container service protocol changes are required for this proposal.

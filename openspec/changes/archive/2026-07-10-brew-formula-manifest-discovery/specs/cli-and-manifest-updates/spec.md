## MODIFIED Requirements

### Requirement: Optional manifest path for apply, build, and plan
[From: local-control-plane-cli]
The `--manifest` flag SHALL be optional for `vcpe apply`/`up`, `vcpe build`, and `vcpe plan`. When omitted, the system SHALL attempt manifest discovery before flag validation.

If exactly one manifest is discovered: the command proceeds as if `--manifest <path>` was provided.
If zero manifests are discovered: error "no manifests found in search path; provide `--manifest` or run `vcpe manifest list`".
If two or more manifests are discovered: error listing discovered names with "specify `--manifest <name>`".

The `--manifest` flag continues to accept: absolute paths, relative paths (containing `/` or ending in `.yaml`), and bare names (no path separators, no `.yaml`). For bare names, the system searches discovery directories for `<name>.yaml`. Path-like values that do not exist on disk return a file-not-found error (no discovery fallback).

#### Scenario: --manifest omitted, single manifest available
- **WHEN** `vcpe apply` is run without `--manifest` and exactly one manifest exists in the search path
- **THEN** the command proceeds using the discovered manifest (logged at DEBUG level)

#### Scenario: --manifest omitted, multiple manifests
- **WHEN** `vcpe apply` is run without `--manifest` and multiple manifests are discovered
- **THEN** the command errors listing the discovered names and instructs the user to specify `--manifest <name>`

#### Scenario: --manifest omitted, no manifests
- **WHEN** `vcpe apply` is run without `--manifest` and no manifests are discovered
- **THEN** the command errors with "no manifests found; run `vcpe manifest list`"

#### Scenario: --manifest bare name
- **WHEN** `vcpe apply --manifest single-gateway` is run
- **THEN** the system searches discovery directories for `single-gateway.yaml` and uses it

#### Scenario: --manifest path-like value that doesn't exist
- **WHEN** `vcpe apply --manifest ./missing.yaml` is run
- **THEN** a file-not-found error is returned (no discovery fallback attempted)

## MODIFIED Requirements

### Requirement: Manifest metadata annotations field
[From: desired-state-manifests]
The manifest `Metadata` block MAY contain an optional `annotations` map. All existing manifests without `annotations` remain valid.

```yaml
metadata:
  name: single-gateway
  labels: {}
  annotations:               # optional
    description: "Single gateway with BNG and WebPA"
```

#### Scenario: Manifest with annotations parses correctly
- **WHEN** a manifest includes `metadata.annotations.description`
- **THEN** it parses without error and the description is accessible

#### Scenario: Manifest without annotations parses correctly
- **WHEN** a manifest omits `metadata.annotations` entirely
- **THEN** it parses without error; `Annotations` is nil

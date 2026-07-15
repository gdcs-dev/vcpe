## REMOVED Requirements

### Requirement: Legacy profile import to canonical manifests
**Reason**: The greenfield manifest-driven model removes the legacy profile/`.env` compatibility layer; the manifest is the sole source of desired state.
**Migration**: Author a `vcpe.dev/v1` `Deployment` manifest directly instead of importing legacy env profiles.

### Requirement: Round-trip compatibility reporting
**Reason**: Profile import/export no longer exists, so round-trip mapping and lossy-field reporting are removed.
**Migration**: None — there is no legacy profile to round-trip; manifests are validated directly against the v1 schema.

### Requirement: Go-owned profile and config commands
**Reason**: Profile management commands are removed with the profile compatibility layer.
**Migration**: Manage deployments through v1 manifests and the `--name` deployment selector; there is no `profile` command in the manifest-driven model.

### Requirement: Compatibility snapshots
**Reason**: Profile compatibility export snapshots are removed along with the profile translation capability.
**Migration**: None — control-plane state is stamped with the `vcpe.dev/v1` schema version instead of profile compatibility snapshots.

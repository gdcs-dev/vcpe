## ADDED Requirements

### Requirement: Health transport is control-plane owned
The manifest schema SHALL NOT expose an interface-level health transport or upstream-selection field. The control plane SHALL derive health endpoint publication from registered service health behavior and resolved network management, and operator-declared application ports SHALL remain distinct from control-plane-allocated health mappings.

#### Scenario: Health-capable service needs no transport hint
- **WHEN** a service type provides the standard health endpoint and all of its topology interfaces use container-managed addressing
- **THEN** a valid manifest requires no interface annotation to enable health publication

#### Scenario: Removed healthUpstream field is rejected
- **WHEN** a manifest contains `services[].interfaces[].healthUpstream`
- **THEN** strict manifest validation rejects the unsupported field with no alias, deprecation behavior, or replacement field

#### Scenario: Health mappings remain absent from manifests
- **WHEN** an operator authors a health-capable service
- **THEN** the manifest contains neither a control-plane health port mapping nor a topology interface selected for health transport
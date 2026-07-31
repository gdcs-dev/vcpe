## ADDED Requirements

### Requirement: Per-service build platform override
The system SHALL support an optional `platforms` field on `image` in the manifest service spec. When present, `image.platforms` SHALL be a non-empty list of OS/arch strings in `os/arch` format (e.g., `linux/arm64`, `linux/amd64`). This field overrides the global platform set for all build operations targeting that service's image. When absent or empty, the service inherits the global platform default.

#### Scenario: Service with image.platforms builds only for declared platforms
- **WHEN** a manifest service declares `image.platforms: [linux/arm64]` and the operator runs `vcpe build --manifest <path>` with no `--platform` flag
- **THEN** that service's image is built for `linux/arm64` only, regardless of the global default

#### Scenario: Service without image.platforms inherits global default
- **WHEN** a manifest service has no `image.platforms` field and the operator runs `vcpe build --manifest <path> --platform linux/amd64,linux/arm64`
- **THEN** that service's image is built for `linux/amd64,linux/arm64`

#### Scenario: Mixed platform constraints in the same manifest
- **WHEN** a manifest contains service A with `image.platforms: [linux/arm64]` and service B with no `image.platforms`, and the operator runs `vcpe release --version v0.3.0`
- **THEN** service A's image is built for `linux/arm64` and service B's image is built for the global default (`linux/amd64,linux/arm64`)

#### Scenario: EnsureForApply is unaffected by image.platforms
- **WHEN** `vcpe up` resolves images for a service with `image.platforms: [linux/arm64]`
- **THEN** the image is built or pulled for the native platform (no platform flag is passed); `image.platforms` does not affect the lifecycle phase

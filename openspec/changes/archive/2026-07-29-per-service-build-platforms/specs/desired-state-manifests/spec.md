## ADDED Requirements

### Requirement: Image schema includes optional platforms field
The `image` block in a service spec SHALL accept an optional `platforms` field containing a list of OS/arch strings (e.g., `[linux/arm64]`, `[linux/amd64, linux/arm64]`). When the field is omitted, its value is treated as an empty list and the service inherits platform selection from the invoking command.

#### Scenario: Manifest with image.platforms round-trips through load/validate
- **WHEN** a manifest YAML includes `image.platforms: [linux/arm64]` for a service
- **THEN** `manifest.Load` parses the field into `Image.Platforms = ["linux/arm64"]` without error

#### Scenario: Manifest without image.platforms loads with empty Platforms slice
- **WHEN** a manifest YAML omits `image.platforms` for a service
- **THEN** `manifest.Load` sets `Image.Platforms` to nil or an empty slice with no error

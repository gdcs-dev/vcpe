## MODIFIED Requirements

### Requirement: sync-homebrew-vcpe defaults to release channel with auto-detected version
`sync-homebrew-vcpe` SHALL default to the `release` Homebrew channel. When `VCPE_HOMEBREW_VERSION` is not set, the release channel SHALL auto-detect the version from the latest git tag in the source repository. When `VCPE_HOMEBREW_SHA256` is not set, it SHALL be computed by downloading the tagged archive from GitHub. The Homebrew formula SHALL embed the version string into the binary via `-ldflags` at install time. The formula SHALL pass `-tags homebrew` to `go build` so that developer-only commands (`build`, `push`, `release`) are excluded from the installed binary. The formula SHALL NOT enumerate which commands are excluded — that knowledge resides solely in the Go source.

#### Scenario: Homebrew formula passes build tag
- **WHEN** `brew install vcpe` compiles the binary
- **THEN** the formula invokes `go build -tags homebrew` and the resulting binary does not include the `build`, `push`, or `release` commands

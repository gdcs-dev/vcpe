# homebrew-vcpe

Homebrew tap for `vcpe` — the Go control-plane binary for the vCPE lab harness.

## Install

```bash
brew tap gdcs-dev/vcpe
brew install vcpe
```

For always-latest from the development branch:

```bash
brew install --HEAD vcpe
```

## Usage

After install, deployment manifests are available in Homebrew's pkgshare directory.
List them:

```bash
vcpe manifest list
```

Apply a deployment:

```bash
vcpe apply --manifest single-gateway   # by name (searches pkgshare + ~/.vcpe/manifests/)
vcpe apply                              # auto-select when only one manifest is available
vcpe apply --manifest /path/to/my.yaml # explicit path (unchanged)
```

User-local manifests can be placed in `~/.vcpe/manifests/` or any directory
listed in `VCPE_MANIFEST_DIRS` (colon-separated).

## Channels

| Channel | Branch | Notes |
|---------|--------|-------|
| `main` | `main` | Stable development branch |
| `development` | `development` | Active development branch (default for this tap) |
| `release` | tagged release | Requires `VCPE_HOMEBREW_VERSION` and `VCPE_HOMEBREW_SHA256` |

## Updating the Formula

The sha256 in `Formula/vcpe.rb` is a point-in-time snapshot of the tracked branch.
After significant pushes, refresh it:

```bash
cd /path/to/vcpe
VCPE_HOMEBREW_CHANNEL=development scripts/sync-homebrew-vcpe
```

This downloads the current branch archive, computes the sha256, renders the
formula, and copies it into this tap.

## Release Validation

Before syncing formula updates, run the repository release gate:

```bash
make release-gate
```

This repository is synced from the source repository at `gdcs-dev/vcpe` using
the `scripts/sync-homebrew-vcpe` helper so the tap stays aligned with the
main branch.

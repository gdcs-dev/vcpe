# homebrew-vcpe

Homebrew tap for `vcpe` — the Go control-plane binary for the vCPE lab harness.

## Install

Homebrew works on macOS and Linux. If you don't have it yet:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

See [brew.sh](https://brew.sh) for full installation instructions.

Once Homebrew is available:

```bash
brew tap gdcs-dev/vcpe
brew install vcpe
```

## Usage

After install, deployment manifests are available in Homebrew's pkgshare directory.
List them:

```bash
vcpe manifest list
```

Apply a deployment:

```bash
vcpe up --manifest single-gateway   # by name (searches pkgshare + ~/.vcpe/manifests/)
vcpe up                              # auto-select when only one manifest is available
vcpe up --manifest /path/to/my.yaml # explicit path (unchanged)
```

Create a new manifest interactively:

```bash
vcpe manifest build
```

Release a new version (stamps manifest, creates git tag, builds and pushes images):

```bash
vcpe release --manifest manifests/example.yaml --version v0.2.0
```

User-local manifests can be placed in `~/.vcpe/manifests/` or any directory
listed in `VCPE_MANIFEST_DIRS` (colon-separated).

## Channels

| Channel | Branch | Notes |
|---------|--------|-------|
| `release` | tagged release | Stable release **(default)** |
| `main` | `main` | Stable development branch |
| `development` | `development` | Active development branch |

## Updating the Formula

The sha256 in `Formula/vcpe.rb` is a point-in-time snapshot of the tracked branch.
After significant pushes, refresh it:

```bash
cd /path/to/vcpe
scripts/sync-homebrew-vcpe
```

To sync a specific channel:

```bash
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

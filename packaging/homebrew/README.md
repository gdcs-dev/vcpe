# homebrew-vcpe

Homebrew tap for `vcpe`.

`vcpe` is the primary packaged Go operator command and release gate target.

## Install

```bash
brew tap gdcs-dev/vcpe
brew install vcpe
```

If Homebrew requires trust for the custom tap first:

```bash
brew trust gdcs-dev/vcpe
brew install gdcs-dev/vcpe/vcpe
```

## Formula

The formula lives at `Formula/vcpe.rb`.

## Release Validation

Before syncing formula updates, run the repository release gate to ensure direct
`vcpe` command coverage and control-plane integration smokes remain healthy:

```bash
make release-gate
```

This repository is synced from the source repository at `gdcs-dev/vcpe` using
the `scripts/sync-homebrew-vcpe` helper so the tap stays aligned with the main
project.
# Runbook

The operator commands stay at the repo root. Their implementations now live under
`platform/` and `services/bng/`.

## Orchestrated Default Deployment

The highest-level workflow now goes through `vcpe`, which manages user
config in `~/.config/vcpe/` and starts the default profile of
`bng-7 + webpa + mv1-7`.

```bash
./scripts/vcpe init
./scripts/vcpe up
./scripts/vcpe status
```

Useful follow-up commands:

```bash
./scripts/vcpe logs bng
./scripts/vcpe logs webpa
./scripts/vcpe config show
./scripts/vcpe profile list
./scripts/vcpe profile show
./scripts/vcpe down
```

## Profile Management

Profiles live in `~/.config/vcpe/profiles/` and are simple env files.
You can create and select them through the orchestrator instead of editing the
main config manually.

```bash
./scripts/vcpe profile create bng-9 default
./scripts/vcpe profile set bng-9 DEPLOY_BNG_CUSTOMER_ID 9
./scripts/vcpe profile set bng-9 DEPLOY_MV1_CUSTOMER_ID 9
./scripts/vcpe profile use bng-9
./scripts/vcpe config show
```

Built-in profiles are copied into `~/.config/vcpe/profiles/` on first
`vcpe init`. Current templates include `default`, `bng-9`, `bng-20`,
and `xb10-7`.

## Homebrew Tap Sync

The repo includes a tap helper that can create a local tap layout and render
the formula into it.

```bash
./scripts/homebrew-tap init gdcs-dev/vcpe
./scripts/homebrew-tap sync gdcs-dev/vcpe
brew trust gdcs-dev/vcpe
brew install gdcs-dev/vcpe/vcpe
```

If you also have the `homebrew-vcpe` tap repository checked out next to this
repo, you can sync the tap files directly into that checkout with:

```bash
./scripts/sync-homebrew-vcpe
```

For tagged releases, set the release metadata before syncing:

```bash
export VCPE_HOMEBREW_CHANNEL=release
export VCPE_HOMEBREW_VERSION=0.1.0
export VCPE_HOMEBREW_SHA256=<archive-sha256>
./scripts/homebrew-tap sync gdcs-dev/vcpe
```

## Build And Start

```bash
./scripts/net setup
./scripts/net verify
./scripts/bng build
./scripts/bng render 7
./scripts/bng up 7
./scripts/bng status 7
./scripts/bng logs 7
```

## Smoke Checks

```bash
./tests/smoke/net-verify.sh
./tests/smoke/bng-7.sh
```

## Stop And Cleanup

```bash
./scripts/bng down 7
./scripts/net cleanup
```

## Out Of Scope

- MV migration
- client migration
- scene orchestration
- graphing and topology tooling
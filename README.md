# Podman vCPE

Phase-1 Podman replacement for the LXD-based BNG workflow in
`meta-lxd-master`. This project currently covers BNG image build, config
render, host networking, lifecycle management for `bng-7`, `bng-9`, and
`bng-20`, plus first-pass `mv1-r21-{7,9,20}` containers that connect to the
BNG using the customer-specific link model currently available in Podman, plus
`xb10-{7,9,20}` gateway simulator containers built locally from a thin wrapper
image on top of `localhost/xb10-dev`, tagged for publish as `ghcr.io/gdcs-dev/xb10:dev`.

The repo is split into a shared platform layer and a BNG service layer:

- `platform/`: host bridge, NAT, and Podman runtime integration
- `services/bng/`: BNG image, compose stack, customer metadata, and smoke tests
- `services/mv1/`: first-pass mv1 image, per-customer config, compose stack,
  and smoke tests
- `services/xb10/`: XB10 gateway simulator compose stack with a local wrapper
    image that normalizes interface names onto the mv1 customer network contract
- top-level `scripts/` and `tests/smoke/`: compatibility shims that preserve the current operator commands

## Homebrew

The repo now includes a Homebrew formula at `Formula/vcpe.rb`
and a top-level `vcpe` orchestrator. The intended packaged install model
is:

```bash
brew install podman
brew install ./Formula/vcpe.rb
vcpe init
vcpe up
```

`podman` is an explicit Homebrew dependency of the formula. On macOS you still
need a running Podman machine before starting the deployment:

```bash
podman machine init
podman machine start
```

The default deployment profile is `bng-7`, `webpa`, and `mv1-7`. User config is
created under `~/.config/vcpe/` on first `vcpe init`.
The current formula tracks the `main` branch until tagged release artifacts are
published.

To bootstrap a tap from this repo and sync the formula into it:

```bash
./scripts/homebrew-tap init gdcs-dev/vcpe
./scripts/homebrew-tap sync gdcs-dev/vcpe
brew trust gdcs-dev/vcpe
brew install gdcs-dev/vcpe/vcpe
```

To sync a checked-out `homebrew-vcpe` repository next to this repo:

```bash
./scripts/sync-homebrew-vcpe
```

## Scope

- Included: host bridge setup, Podman image build, customer render, BNG startup,
  smoke verification, first-pass `mv1-r21-{7,9,20}` direct BNG peers, and
  `xb10-{7,9,20}` gateway simulator peers built from a local wrapper image.
- Excluded: full MV feature parity, client containers, scene orchestration,
  graphing, and full topology migration.

## Prerequisites

- macOS host with rootful Podman installed
- `podman-compose`
- `python3`
- `sudo` access for bridge and firewall setup

## Quick Start

For the new orchestrated path:

```bash
./scripts/vcpe init
./scripts/vcpe up
./scripts/vcpe status
```

The lower-level per-service commands remain available:

```bash
./scripts/net setup
./scripts/bng build
./scripts/bng render 7
./scripts/bng up 7
./scripts/mv1 build
./scripts/mv1 up 7
./scripts/xb10 build
./scripts/xb10 up 7
./scripts/bng status 7
```

To push the repo-built images to GHCR, export `GHCR_USERNAME` and `GHCR_TOKEN`
or `GITHUB_USERNAME` and `GITHUB_TOKEN`, then run `./scripts/bng push`,
`./scripts/mv1 push`, `./scripts/webpa push`, or `./scripts/xb10 push`.

To inspect or change the packaged config:

```bash
./scripts/vcpe config show
./scripts/vcpe config set VCPE_PULL_POLICY always
./scripts/vcpe profile list
./scripts/vcpe profile show
./scripts/vcpe profile create bng-9 default
./scripts/vcpe profile set bng-9 DEPLOY_BNG_CUSTOMER_ID 9
./scripts/vcpe profile set bng-9 DEPLOY_MV1_CUSTOMER_ID 9
./scripts/vcpe profile use bng-9
```

Built-in deployment profiles currently include `default`, `bng-9`, `bng-20`,
and `xb10-7`.

See `docs/runbook.md` for the full operator workflow.
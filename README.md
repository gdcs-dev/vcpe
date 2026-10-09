# Podman vCPE

vCPE is a local development and testing environment for containerized broadband
components (BNG, GATEWAY, WebPA, WebConfig, XB10, and client peers) on Podman.

The only operator command is `vcpe` (the Go control plane). It reconciles a
declarative desired-state manifest into Podman projects. Top-level scripts in
`scripts/` are retired stubs and are not a working operator path.

## Project Layout

- `controlplane/`: Go control-plane binaries and packages (the `vcpe` operator)
- `services/`: curated compose files and service assets per service type
- `platform/`: host networking helpers and shared shell libs
- `docs/`: architecture, networking, and runbook documentation

Health endpoint and status behavior is documented in [docs/health.md](docs/health.md).

## Build

Initialize the telemetry-gateway source submodule after cloning:

```bash
git submodule update --init --recursive services/telemetry-gateway
```

Developers whose `~/.ssh/config` selects the required GitHub identity through
the `github-gdcs` host alias can keep the canonical repository URL and add a
workstation-local rewrite:

```bash
git config --local url."git@github-gdcs:".insteadOf "git@github.com:"
git submodule sync -- services/telemetry-gateway
```

```bash
cd controlplane
go build -o bin/vcpe ./cmd/vcpe
```

You can also run directly: `go run ./controlplane/cmd/vcpe/main.go`.

On macOS, initialize and start a Podman machine once:

```bash
podman machine init
podman machine start
```

Wireless development requires the dynamic hwsim machine image:

```bash
podman machine init \
	--image docker://ghcr.io/gdcs-dev/vcpe/machine-os:6.1
podman machine set --rootful
podman machine start
```

This image intentionally boots with no wireless PHYs. The vCPE control plane
allocates and attaches declared radios through `/usr/libexec/vcpe/vcpe-hwsim`
after their target containers start.

Build and run the maintained tri-band, 24-VAP WPA3-Personal example with:

```bash
vcpe build --manifest manifests/dev/wireless.yaml
vcpe up --manifest manifests/dev/wireless.yaml
vcpe status --name wireless
vcpe down --name wireless
```

The three stations use `services/wireless-client` through the existing
`generic-container` type; `wireless-client` is an image, not a registered
service type. The development passphrase must contain 8 through 63 printable
ASCII characters. This development-only manifest carries a literal credential
so it runs without environment setup; use an `env` or `file` provider for
non-development deployments. Credentials are not written to ordinary generated
artifacts.

For host-network reconciliation on macOS, `vcpe` auto-detects and delegates Linux
network commands to the Podman machine host. You can force delegation explicitly:

```bash
export VCPE_HOSTNET_DELEGATED=1
```

## Quick Start

Discover bundled manifests or create one interactively:

```bash
vcpe manifest list                      # see available manifests
vcpe manifest build                     # interactive wizard — create a new manifest
```

Bring up the full example deployment:

```bash
vcpe up --manifest manifests/example.yaml
vcpe status --name example
vcpe logs   --name example
vcpe down   --name example
```

Manifests use the `vcpe.dev/v1` `Deployment` schema. The deployment identity is
`metadata.name`; `customer` is at most an opaque label under `metadata.labels`.

If you change a deployment in a disruptive way (for example a network CIDR
change), acknowledge it explicitly:

```bash
vcpe up --manifest manifests/example.yaml --allow-disruptive
```

## Commands

| Command | Description |
|---------|-------------|
| `vcpe build` | Build or pull service images from a manifest |
| `vcpe push` | Push service images to their registries |
| `vcpe stamp` | Pin image tags in one or more manifests to a version (no git ops) |
| `vcpe release` | git commit+tag+push pre-stamped manifests, then build and push images |
| `vcpe up` | Bring up a deployment from a manifest (alias: `apply`) |
| `vcpe plan` | Show planned changes without applying |
| `vcpe down` | Tear down a named deployment (alias: `destroy`, requires `--force`) |
| `vcpe list` | List known deployments |
| `vcpe manifest list` | Discover and list available manifest files |
| `vcpe manifest build` | Interactive wizard to create or update a manifest |
| `vcpe status` | Show control-plane status for a deployment |
| `vcpe logs` | Show operation timeline and container logs |
| `vcpe config` | Show effective configuration |
| `vcpe state` | Manage persisted control-plane state |
| `vcpe version` | Print the embedded vcpe version |

Run `vcpe <command> --help` for full flag reference.

## Service Types

Each service declares a `type` that selects a registered behavior. The `config`
block is decoded strictly against that type's schema; unknown fields are rejected
before apply.

| Type | Description |
|------|-------------|
| `bng` | Broadband Network Gateway — DHCP, DNS, iptables NAT |
| `gateway` | CPE simulator — WAN DHCP client, LAN bridge, NAT |
| `webpa` | WebPA / WebConfig device-management server |
| `webconfig` | Development-only WebConfig server for Gateway's bundled `webcfg` client |
| `event-sink` | XMiDT webhook consumer; logs matching WRP events as structured JSON |
| `telemetry-gateway` | Embedded local telemetry gateway, PostgreSQL, and observability stack |
| `xb10` | XB10 CPE simulator |
| `oktopus` | Oktopus USP controller |
| `generic-container` | Arbitrary container with configurable command |

New workloads are added by registering a new service type in the control plane,
not by editing the planner or renderer.

The embedded telemetry-gateway development topology is documented in
[docs/telemetry-gateway.md](docs/telemetry-gateway.md).

### WebConfig Development Service

`webconfig` supplies Gateway's bundled `webcfg` client with MessagePack
subdocuments from `http://webconfig:9000/api/v1/device/{mac}/config`. It uses
ephemeral in-container SQLite state, seeds its development fixture on every
start, and has authentication and external connectors disabled. It is only for
isolated local development and testing; it MUST NOT be used as a production
deployment pattern.

## Network Fields

Networks support optional `driver` (default: `bridge`), `driverOptions` (e.g.
`parent:` for macvlan), and `ipamDriver`. When `ipamDriver: none` is set, Podman
does not assign IPs to containers on that network — container entrypoints assign
IPs from the explicit `interfaces[].ipv4` values in the manifest.

## State Schema Cutover

Persisted state is stamped with `schemaVersion: vcpe.dev/state/v3`. The v2
wireless snapshot shape is incompatible; there is no automatic conversion.
Tear down active deployments with the old binary, then use the v3 binary to
reset and re-stamp the root before applying new manifests:

```bash
controlplane/bin/vcpe state reset
```

## Optional Makefile Wrappers

The top-level Makefile provides optional convenience wrappers. It does not own
or redefine orchestration behavior.

```bash
make help
make build
make release-gate
```

## Troubleshooting

- Inspect resolved control-plane config: `vcpe config show`
- View desired/planned/observed state: `vcpe status --name <deployment> --json`
- View operation context + container logs: `vcpe logs --name <deployment>`

For deeper procedures, see `docs/runbook.md`.

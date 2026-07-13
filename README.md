# Podman vCPE

vCPE is a local development and testing environment for containerized broadband
components (BNG, GATEWAY, routerd, WebPA, XB10, and client peers) on Podman.

The only operator command is `vcpe` (the Go control plane). It reconciles a
declarative desired-state manifest into Podman projects. Top-level scripts in
`scripts/` are retired stubs and are not a working operator path.

## Project Layout

- `controlplane/`: Go control-plane binaries and packages (the `vcpe` operator)
- `services/`: curated compose files and service assets per service type
- `platform/`: host networking helpers and shared shell libs
- `tests/smoke/`: control-plane smoke scenarios
- `docs/`: architecture, networking, and runbook documentation

## Prerequisites

- macOS or Linux with Podman available
- `podman-compose`
- Go toolchain for local binary builds or `go run`

On macOS, initialize and start a Podman machine once:

```bash
podman machine init
podman machine start
```

For host-network reconciliation on macOS, `vcpe` auto-detects and delegates Linux
network commands to the Podman machine host. You can still force delegation
explicitly if needed:

```bash
export VCPE_HOSTNET_DELEGATED=1
```

## Build vcpe

```bash
cd controlplane
go build -o bin/vcpe ./cmd/vcpe
```

You can run `vcpe` as a built binary (`controlplane/bin/vcpe`) or via
`go run ./controlplane/cmd/vcpe/main.go`.

## Quick Start

Author a `vcpe.dev/v1` `Deployment` manifest. The deployment identity is
`metadata.name`; `customer` is at most an opaque label under `metadata.labels`.
A manifest is required by `build`, `plan`, `apply`, and `up`:

```bash
cat > ./manifest-bng-7.yaml <<'EOF'
apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: bng-7
  labels:
    customer: "7"
spec:
  maxReplicasPerService: 3
  maxActiveDeployments: 10
  networks:
    - role: mgmt
      ipv4: { cidr: 10.10.10.0/24, gateway: 10.10.10.1, pool: { start: 10.10.10.10, end: 10.10.10.250 } }
    - role: wan
      nat: true
      firewall: true
      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1, pool: { start: 10.7.200.10, end: 10.7.200.250 } }
    - role: cm
      ipv4: { cidr: 10.7.201.0/24, gateway: 10.7.201.1, pool: { start: 10.7.201.10, end: 10.7.201.250 } }
  services:
    - name: bng
      type: bng
      replicas: 1
      image: { repository: ghcr.io/gdcs-dev/bng, tag: dev, pullPolicy: build-if-missing }
      interfaces:
        - { role: mgmt }
        - { role: wan, defaultRoute: true }
        - { role: cm }
      config:
        access:
          - role: wan
            dhcp4:
              subnet: 10.7.200.0/24
              ranges:
                - { start: 10.7.200.100, end: 10.7.200.200 }
              options: { routers: 10.7.200.1 }
              leaseSeconds: 3600
EOF
```

```bash
controlplane/bin/vcpe init
controlplane/bin/vcpe up --manifest ./manifest-bng-7.yaml
controlplane/bin/vcpe status --name bng-7
controlplane/bin/vcpe logs --name bng-7
controlplane/bin/vcpe down --name bng-7
```

Deployment-targeting commands (`status`, `logs`, `down`, `destroy`, `service`)
select a deployment by `--name <metadata.name>`.

If you change a deployment in a disruptive way (for example a network CIDR
change), acknowledge it explicitly:

```bash
controlplane/bin/vcpe up --manifest ./manifest-bng-7.yaml --allow-disruptive
```

## Service Types

Each service declares a `type` that selects a registered behavior: `bng`, `gateway`,
`webpa`, or `generic-container`. A type's `config` block is decoded strictly
against that type's schema; unknown fields are rejected before apply. New
workloads are added by registering a new service type in the control plane, not
by editing the planner or renderer.

## State Schema Cutover

Persisted state is stamped with `schemaVersion: vcpe.dev/v1`. If you run against
a state root written by an incompatible schema, `vcpe` refuses to operate and
directs you to reset it:

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

## Smoke Checks

```bash
./tests/smoke/vcpe-primary-status.sh
./tests/smoke/vcpe-service-coverage.sh
./tests/smoke/controlplane-bng-7.sh
./tests/smoke/controlplane-bng-20.sh
```

## Troubleshooting

- Inspect resolved control-plane config: `controlplane/bin/vcpe config show`
- View desired/planned/observed state: `controlplane/bin/vcpe status --name bng-7 --json`
- View operation context + container logs: `controlplane/bin/vcpe logs --name bng-7`

For deeper procedures, see `docs/runbook.md`.

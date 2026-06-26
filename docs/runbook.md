# Runbook

The only operator command is the Go `vcpe` binary. Script paths under
`scripts/` are retired stubs and are not a working run path.

When running from macOS, delegated host-network mode is auto-detected so
bridge/NAT/firewall reconciliation executes inside the Podman machine Linux
host. You can still set it explicitly:

```bash
export VCPE_HOSTNET_DELEGATED=1
```

## Primary Command Path

Build once:

```bash
cd controlplane
go build -o bin/vcpe ./cmd/vcpe
```

Author a `vcpe.dev/v1` manifest (identity is `metadata.name`):

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

controlplane/bin/vcpe init
controlplane/bin/vcpe up --manifest ./manifest-bng-7.yaml
controlplane/bin/vcpe status --name bng-7
```

Useful follow-up commands:

```bash
controlplane/bin/vcpe logs --name bng-7
controlplane/bin/vcpe config show
controlplane/bin/vcpe down --name bng-7
```

### Rollback Guidance

- Inspect state directly:

```bash
controlplane/bin/vcpe status --name bng-7
```

- Review operation timeline and drift summary:

```bash
controlplane/bin/vcpe status --name bng-7 --json
```

## Deployment Selection And Scaling Limits

- Deployment-targeting commands (`status`, `logs`, `down`, `destroy`, `service`)
  identify a deployment by `--name <metadata.name>`. An unknown name is reported
  as an error.
- The active-deployment cap is `spec.maxActiveDeployments`, counting distinct
  active `metadata.name` values. Applying a new deployment beyond the cap fails
  with a cap-violation error.
- Per-service replica count is bounded by `spec.maxReplicasPerService`.

## State Schema-Version Cutover

Persisted state is stamped `schemaVersion: vcpe.dev/v1`. A state root written by
an incompatible schema is refused with an actionable error. Reset and re-stamp
the root before applying v1 manifests (there is no automatic migration):

```bash
controlplane/bin/vcpe state reset
```

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
controlplane/bin/vcpe build --manifest ./manifest-bng-7.yaml
controlplane/bin/vcpe up --manifest ./manifest-bng-7.yaml
controlplane/bin/vcpe status --name bng-7
controlplane/bin/vcpe logs --name bng-7
```

## Smoke Checks

```bash
./tests/smoke/vcpe-primary-status.sh
./tests/smoke/vcpe-service-coverage.sh
./tests/smoke/controlplane-bng-7.sh
./tests/smoke/controlplane-bng-20.sh
```

## Release Gate

```bash
make release-gate
```

This target enforces direct `vcpe` command coverage and control-plane integration
smokes before release packaging.

## Stop And Cleanup

```bash
controlplane/bin/vcpe down --name bng-7
```

## Out Of Scope

- MV migration
- scene orchestration
- graphing and topology tooling

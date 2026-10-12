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

For a deployment containing Gateway or XB10 plus WebPA, inspect the complete
connection path without reading container logs:

```bash
controlplane/bin/vcpe diagnose --name example-full --from gateway --to webpa --client-service apparmor-simulator
controlplane/bin/vcpe diagnose --name example-full --from gateway --to webpa --client-service my-test-app --json
```

To inspect deployment-local DNS from a management service:

```bash
podman inspect example-webpa-1 --format '{{json .HostConfig.Dns}}'
podman exec example-webpa-1 getent hosts gateway gateway-1 gateway-1-wan
podman exec example-webpa-1 ip route get "$(podman exec example-webpa-1 getent hosts gateway | awk 'NR == 1 { print $1 }')"
podman exec example-bng-1 cat /etc/dnsmasq.dhcp-hosts.map
podman exec example-bng-1 cat /etc/dnsmasq.dhcp.hosts
podman exec example-bng-1 cat /var/log/dhcpd-notify.log
```

The first DNS entry in Podman metadata should be BNG's management address and
the second should be the management-network gateway used by Aardvark. Podman
may still show only Aardvark in the container's `/etc/resolv.conf`; Netavark
applies the configured upstream chain behind that listener.

The route lookup for a Gateway or XB10 access address should report BNG's
management address as its next hop. WebPA installs these routes before starting
Caduceus so resolved webhook destinations are reachable over TCP as well as
DNS.

If a Gateway or XB10 alias does not resolve, verify that its lease is active,
its MAC appears in `dnsmasq.dhcp-hosts.map`, and the corresponding address and
aliases appear in `dnsmasq.dhcp.hosts`. Client-provided DHCP hostnames are
diagnostic only and do not control published names.

To send one explicit, bounded diagnostic event from a supported Gateway or XB10
source through WebPA to an event-sink callback, use:

```bash
controlplane/bin/vcpe diagnose --name example-full --from gateway --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event apparmor/diagnostic \
  --device-id mac:001122334455 --json

controlplane/bin/vcpe diagnose --name xb10 --from xb10 --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event devices/diagnostic \
  --device-id mac:001122334455 --json
```

This is not production-event tracing. It emits at most one reserved marker only
after CPE, Talaria, subscriber, and Argus prerequisites pass. A missing routing
record or callback receipt is inconclusive, not proof of delivery or rejection.

`failed` identifies the first confirmed broken boundary. `unknown` means the
available signal was not authoritative; downstream checks continue only when
that edge is explicitly non-blocking. `skipped` means a blocking prerequisite
was unresolved. A failed or inconclusive graph is printed and returns a nonzero
exit status, so scripts should capture output before checking the status.

### Rollback Guidance

- Inspect state directly:

```bash
controlplane/bin/vcpe status --name bng-7
```

- Review operation timeline and drift summary:

```bash
controlplane/bin/vcpe status --name bng-7 --json
```

For wireless deployments, selected-deployment status also shows the allocated
group bit, each radio's manager name, device, medium, and reconciliation status,
and every planned AP VAP's slot, interface, BSSID, bridge, and observed readiness
(`ready`, `missing`, `mismatch`, or `unknown` if the container cannot be probed). A
`failed:` or `release-failed:` status is retained for retry. Run `vcpe down`
again after correcting manager or machine availability; do not delete the state
root manually.

```bash
controlplane/bin/vcpe status --name wireless --json
```

The `wireless.topology[]` entries include `medium`, `band`, `channel`,
`widthMHz`, and `vaps[]` (`slot`, `interface`, `bssid`, `bridge`, `state`).
Station radios have an empty `vaps` list. No profile credential or protected
file contents are included.

### Wireless Personal Credentials

WPA2-Personal and WPA3-Personal networks use `passphraseSecretRef`; the value is
resolved from the referenced secret provider during apply. The maintained
development manifest is explicitly labeled `environment: development` and uses
a development-only `literal` provider, so it applies without shell setup:

```bash
controlplane/bin/vcpe up --manifest manifests/dev/wireless.yaml
```

Use an `env` or `file` provider for manifests outside local development.

Values must contain 8 through 63 printable ASCII characters. Changing the
resolved value and applying again recreates only AP and station consumers, so a
brief WLAN interruption is expected. If recreation fails, keep the environment
value available and retry apply, or run `vcpe down --name <deployment>`; the
control plane retains the new protected value and radio ownership for recovery.

To return a network to open association, set `security: open`, remove
`passphraseSecretRef`, and apply. Successful down removes that deployment's
protected files after consumers stop. `state reset` stops known consumers,
cleans all manager-owned radios, removes every protected credential, and then
clears state. Status, logs, and command arguments never display credential
values.

## Deployment Selection And Scaling Limits

- Deployment-targeting commands (`status`, `diagnose`, `logs`, `down`, `destroy`, `service`)
  identify a deployment by `--name <metadata.name>`. An unknown name is reported
  as an error.
- The active-deployment cap is `spec.maxActiveDeployments`, counting distinct
  active `metadata.name` values. Applying a new deployment beyond the cap fails
  with a cap-violation error.
- Per-service replica count is bounded by `spec.maxReplicasPerService`.

## Routerd Removal

Routerd is no longer a supported service type. New and previously deployed
manifests with `type: routerd` fail `vcpe plan` and `vcpe up` preflight without
changing running workloads. Do not substitute Gateway or `generic-container`
without separately validating the network topology and configuration.

Before upgrading, run `vcpe list` and take down any routerd deployment whose
saved generated Compose artifacts are missing or uncertain using the previous
binary:

```bash
vcpe down --name <deployment-name>
```

After upgrading, `vcpe down --name <deployment-name>` still removes an existing
routerd deployment if its saved per-service `compose.yaml` is present. If the
artifact is missing, teardown reports an error instead of claiming success:
restore the saved Compose artifact or use the previous compatible binary to
take down the deployment. Do not reset state as a substitute for stopping the
running project. Replace the old manifest with a separately validated supported
service type before applying again.

## State Schema-Version Cutover

Persisted state is stamped `schemaVersion: vcpe.dev/state/v3`. Existing v2
desired snapshots use the combined wireless-network shape and cannot be safely
reinterpreted. A v2 or other incompatible state root is refused; there is no
automatic migration. Tear down deployments with the v2 binary while it can
still read its records. Then use the v3 binary to reset and re-stamp the root
before applying the new media/profile/VAP manifests:

```bash
controlplane/bin/vcpe state reset
```

Reset invokes manager GC with an explicitly empty keep set before deleting any
database rows. If manager initialization or cleanup fails, reset aborts and
leaves leases, snapshots, radio records, and group allocations intact. Ensure
the hwsim-enabled Podman machine is running before reset on macOS.

Rollback to a v2 binary requires tearing down all deployments with the v3
binary, confirming `vcpe-hwsim list` is empty, then resetting with the v2
binary before another apply. Never reuse a v3 state root with a v2 binary. A reset destroys
all desired snapshots and allocations; preserve any needed manifest source
outside the state root before starting either cutover.

Old wireless manifests must move channel/band/width into `wirelessMedia[]`, keep
SSID/security in reusable `wirelessNetworks[]`, move AP bridge/profile selection
to `radios[].vaps[]`, and give each station a medium/profile pair. To secure a
profile, add a `spec.secrets[]` env/file declaration and set it to
`wpa2-personal` or `wpa3-personal`, and reference only its name through
`passphraseSecretRef`. A literal passphrase is permitted only in a manifest
explicitly labeled `metadata.labels.environment: development`. Do not migrate
production credentials into manifests, command arguments, Compose environment,
or persisted desired-state fields.

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

This target runs control-plane tests and the sandboxed DHCP-to-DNS lifecycle
test. The deployed cross-network DNS smoke is included but opt-in because it
rebuilds BNG and provisions a Podman deployment:

```bash
VCPE_RUN_CROSS_NETWORK_DNS=1 make release-gate
```

## Publishing A Release

The release workflow is a two-step process that separates stamping from the git tag, allowing you to test pinned images across multiple manifests before the tag is created.

**Step 1 — Stamp all manifests** (no git operations):

```bash
# Stamp a single manifest
vcpe stamp --version v0.3.0 --manifest manifests/example.yaml

# Stamp multiple manifests explicitly
vcpe stamp --version v0.3.0 \
  --manifest manifests/example.yaml \
  --manifest manifests/example-macvlan.yaml

# Stamp all manifests in a directory with a glob
vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml"
```

After stamping, test your deployments:

```bash
vcpe up --manifest manifests/example.yaml
# verify, then:
vcpe down --manifest manifests/example.yaml
```

**Step 2 — Publish the release** (git commit + tag + push + build + push images):

```bash
# Auto-detects stamped manifests via git diff
vcpe release --version v0.3.0

# Or provide explicit manifest paths
vcpe release --version v0.3.0 --manifest "manifests/*.yaml"
```

`vcpe release` verifies that every first-party image tag in each manifest equals `--version` before touching git. If any manifest was missed by `stamp`, it will fail with a clear error naming the offending file and service.

Both `vcpe stamp` and `vcpe release` must be run from the `main` branch.

## Stop And Cleanup

```bash
controlplane/bin/vcpe down --name bng-7
```

## Out Of Scope

- MV migration
- scene orchestration
- graphing and topology tooling

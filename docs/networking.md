# Networking

## Manifest-declared networks

Topology comes from `spec.networks[]`. Each network has a `role`, an optional
`bridge` (default `<metadata.name>-<role>`), `nat`/`firewall` flags, and optional
`ipv4` and `ipv6` blocks. Each address-family block may define a `cidr`, an
optional `gateway` (default: first usable host in the CIDR), and an optional
`pool` range. A network may declare either or both families, supporting
IPv4-only, IPv6-only, or dual-stack:

```yaml
spec:
  networks:
    - role: wan
      bridge: edge-wan        # optional; default <metadata.name>-<role>
      nat: true
      firewall: true
      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1, pool: { start: 10.7.200.10, end: 10.7.200.250 } }
      ipv6: { cidr: "2001:dae:7:1::/64" }
```

## Interfaces

Services attach to networks through `services[].interfaces[]`. An interface binds
to a network by `role`, with an optional `device` (default: ordered `eth<n>`), an
optional `mac`, and at most one `ipv4` and one `ipv6` address. The gateway is
inherited from the network. Explicit `mac`/`ipv4`/`ipv6` are valid only when
`replicas: 1`; with `replicas > 1`, IPAM allocates per replica and MACs are
indexed.

At most one interface per service may set `defaultRoute: true`; if none does, the
interface bound to the `wan` role is the default route.

## Simulated wireless media and radios

RF media are declared under `spec.wirelessMedia[]`; SSID/security profiles are
declared under `spec.wirelessNetworks[]`. Neither is a Podman network or consumes
IPAM leases. Services attach with `radios[]`: an AP radio selects a medium and
one to eight VAP slots (including slot 0), each selecting a profile and a bridge;
a station selects exactly one medium and profile. The planner derives stable
manager names, primary MACs, VAP interfaces, and BSSIDs. Only primary radios
are manager-owned; hostapd creates secondary BSS interfaces.

2.4 and 5 GHz support `open`, `wpa2-personal`, and `wpa3-personal`. WPA2 uses
RSN, WPA-PSK, and CCMP. WPA3 uses SAE, CCMP,
and required protected management frames; transition mode is not enabled. A
personal network references a unique `spec.secrets[].name` through
`passphraseSecretRef`:

```yaml
spec:
  wirelessMedia:
    - {name: rf24, band: 2.4ghz, channel: 1, widthMHz: 20}
    - {name: rf5, band: 5ghz, channel: 36, widthMHz: 20}
    - {name: rf6, band: 6ghz, channel: 5, widthMHz: 20}
  wirelessNetworks:
    - name: home
      ssid: vcpe-lab
      security: wpa3-personal
      passphraseSecretRef: home-wifi
  secrets:
    - name: home-wifi
      provider: literal
      value: vcpe-development-only
  services:
    - name: gateway
      bridges: [{name: brlan0}]
      radios:
        - {name: ap24, medium: rf24, device: wlan0, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}
        - {name: ap5, medium: rf5, device: wlan1, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}
        - {name: ap6, medium: rf6, device: wlan2, mode: ap, vaps: [{slot: 0, network: home, bridge: brlan0}]}
```

The resolved credential must contain 8 through 63 printable ASCII characters.
The `literal` provider is accepted only when
`metadata.labels.environment: development` is set and is intended for local
development manifests. For other deployments, use an `env` provider or the
`file` provider's `path:entry` key syntax; do not put values in command
arguments.

A fixed station's `(medium, profile)` pair must have exactly one AP VAP. A
roaming station instead declares `roaming.media[]` with two or more eligible
AP media for one profile and no fixed `medium`. AP VAPs name
bridges owned by the same service; station radios use DHCP, do not name a bridge,
and may request the service's one default route. Radios are attached after Compose starts, and
workload startup waits for their devices before network initialization.

Changing a profile value, security mode, VAP assignment, or bridge briefly
interrupts its consumers: a Gateway using that profile restarts once, as do
affected stations. PHY identities and group ownership are retained. Changing
a medium or primary radio identity is disruptive and requires approval. Open
association is supported only on 2.4/5 GHz; remove `passphraseSecretRef` when
changing an eligible profile to `security: open`. Successful down
and state reset remove protected runtime credential files after consumers stop.

### Two-node mesh and roaming example

The [two-Gateway manifest](../manifests/dev/mesh-roaming.yaml) declares one
root DHCP bridge and one unnumbered remote bridge. Both are joined only by
dedicated SAE 802.11s mesh radios; each owns a separate access PHY on the
same 5 GHz channel (36). The mesh backhaul uses 2.4 GHz channel 6 on the
tested machine. A fixed diagnostic client on `remote-access` proves root
DHCP and LAN reachability over the mesh, without a wired interface or disturbing
the roaming client. The roaming client selects the shared WLAN profile across
`root-access` and `remote-access`. `wirelessScenarios[].steps[]` specifies
ordered `atMs`, AP VAP `service`/`replica`/`radio`/`slot` references, and
`snrDb` per link. The assertions require the initial root and final remote
BSSIDs, unchanged IPv4, and bounded roam and packet gaps. Normal `vcpe up`
installs a stable RF baseline; it never starts the crossover.

BNG leases distinct WAN addresses to the root and remote Gateways, whose WAN
interfaces stay off their client bridges. WebPA and BNG share a deployment-local
management network with no host-published management port. The smoke plans a
second deployment with separate management and WAN ranges and BNG DHCP settings.
Its live verifier requires both planned WAN-MAC identities in WebPA's inventory
and a correlated device-directed client-list response from each Gateway before
and during mesh loss; one registered or responsive Gateway is insufficient.

The development manifest embeds 8-63 character literal test keys for mesh SAE
and WLAN access. All six services use `build-if-missing` with local build
contexts; `vcpe up` reuses a local `:dev` image if present. Run
`vcpe build --manifest manifests/dev/mesh-roaming.yaml` from the repository root
to rebuild the images from local sources. Development-labeled mesh manifests
may use this local Gateway build when it carries the mesh-SAE capability label;
other mesh deployments require an inspected immutable Gateway digest. Do not reuse
the test keys outside a development environment. Use the supported Podman machine image
`ghcr.io/gdcs-dev/vcpe/machine-os@sha256:1faa9e252d46db00baba5c695c0ed617e772816a55a9c31a0e2cefc42bf24a90`.
Use a fresh state root for this lab by passing the same `--state-root <path>` to plan, up,
scenario run, and down; do not reset an existing deployment's state to run
the example. `vcpe plan --manifest
manifests/dev/mesh-roaming.yaml` checks the topology without running it.
Run `VCPE_MESH_SMOKE_VALIDATE=1 bash tests/smoke/controlplane-mesh-roaming-machine.sh`
to validate both temporary manifest plans offline; this is not a machine proof.
For the separate live machine smoke, use the most recently built local
`ghcr.io/gdcs-dev/gateway:dev` image. Rebuild with
`vcpe build --manifest manifests/dev/mesh-roaming.yaml` to refresh that tag;
`build-if-missing` builds it when no local image exists. The control plane checks
the image's mesh-SAE capability label before allocating radios. Run
`tests/smoke/controlplane-mesh-roaming-machine.sh` with
`VCPE_RUN_MESH_SMOKE=1`; the development manifest already contains the wireless
test keys, so no image digest or key variables are needed. The smoke generates its
own temporary manifests, state root, and two deployments, and fails rather than claiming a live
proof when prerequisites are absent. The normal release gate reports a skip
until explicitly opted in; a skipped smoke is not a passed machine proof.
An opt-in Linux arm64 machine smoke passed on 2026-10-08 using the local
development images: both Gateways completed device-directed WebPA management
before and during mesh loss, with remote-root DHCP and LAN over SAE mesh,
path-failure isolation, automatic same-channel roaming with bounded outage
and stable IPv4, reapply, Gateway restart/repair, neighboring deployment
isolation, and zero-radio cleanup.
This does not verify other architectures, upstream RF-rate behavior, or
cross-channel roaming.
On macOS, verify that the Podman VM boots with
`ostree.prepare-root.composefs=0` and its `private.mount` and `Users.mount`
units are active. Without that kernel argument, the read-only composefs root
cannot create `/private` or `/Users`, so generated Gateway hostapd files under
the canonical `/private/var/folders` host path cannot be bind-mounted. Fix the
machine boot configuration and restart it; creating a symlink inside the
read-only VM does not work.
After `vcpe up --manifest manifests/dev/mesh-roaming.yaml`, run
`vcpe scenario run --name mesh-roaming --scenario crossover`. It selects the
active persisted snapshot, waits for VAP, peer, remote-root DHCP, and station
readiness, preconditions the initial AP, then plays the timed RF changes. The
run samples the live supplicant/kernel BSSID, IPv4 address, and LAN probes;
success reports the observed BSSID transition, roam duration, unchanged IPv4,
and bounded sampled probe outage. Failure, cancellation, and success restore
the scenario's links to the host-wide RF baseline with confirmed per-link
updates; ordinary runs do not restart a healthy machine-wide daemon. A failed
link reset triggers global baseline recovery, which can briefly disrupt other
deployments. Leave `VCPE_DAEMON_SOCKET` unset for this
local command. Do not issue `wpa_cli roam`, access the station supplicant
socket, or modify RF links during the experiment: known outside intervention
invalidates it. Passive samples cannot prove that a privileged outside actor
did not interfere. A manifest, plan, or successful apply is not proof of a
roam; this example does not establish upstream RF-rate behavior or
cross-channel RF behavior.

### Tri-band cutover constraints

The media/profile/VAP schema is required; old `wirelessNetworks[].channel` and
AP `radios[].bridge` fields fail strict loading. Up to three independent AP
PHYs and eight VAPs per PHY give 24 VAP slots. The first implementation supports
20 MHz channels only: 2.4 GHz channels 1-14, non-DFS
5 GHz channels 36, 40, 44, 48, 149, 153, 157, 161, and 165, and 6 GHz
preferred-scanning channels 5, 21, 37, 53, 69, 85, 101, 117, 133, 149, 165,
181, 197, 213, and 229. DFS, wider channels, automatic channel selection,
MLO, and WPA2/WPA3 transition mode are unsupported. Six GHz attachments
require WPA3-Personal with SAE, CCMP, and required protected management frames;
they never fall back to open or WPA2-Personal.

Literal secrets remain an explicit development-only exception, gated by
`metadata.labels.environment: development`. For other deployments, use `env`
or `file` secret providers and keep passphrases out of command arguments.

On macOS, the control plane delegates the root-only manager call through the
Podman machine. On Linux, wireless attachments require rootful Podman and root.
WSL and rootless environments are rejected before mutation.

## Deterministic MAC and bridge naming

- **MAC**: an interface without an explicit `mac` gets a deterministic,
  locally-administered MAC derived from `metadata.name/service/role[/index]`
  (`02:` prefix). The planner and runtime-init contract derive it identically.
- **Bridge/interface names**: derived names are capped at the 15-character kernel
  limit (IFNAMSIZ). When `<metadata.name>-<role>` overflows, the name is
  truncated with a deterministic hash suffix; an explicit name over 15 characters
  produces a warning.

## NAT

Networks with `nat: true` are masqueraded via the host default uplink. The
masquerade source CIDR is the network's CIDR; egress uplink is an environment
concern, not a manifest field.

## Deployment-local DNS

BNG dnsmasq bridges names between Podman-managed services and devices that
receive ISC DHCP leases on BNG access networks. Non-BNG services attached to
the same managed `mgmt` network use BNG's allocated management address as their
first DNS upstream and the Podman gateway (Aardvark) as fallback. BNG itself
continues to use Aardvark and forwards management-peer and public queries to it,
avoiding a recursive BNG-to-itself resolver path.

WebPA also installs routes for each non-management IPv4 network attached to
that BNG, using BNG's management address as the next hop. This keeps Caduceus
callbacks and diagnostics on the deployment router path instead of WebPA's
Podman bridge default route. The route destinations and next hop are derived
from the resolved deployment plan; manifests do not require static route data.

DHCP records use planned interface MAC addresses rather than client-supplied
hostnames. For a service named `gateway`, replica one receives these aliases on
its primary access interface:

```text
gateway
gateway-1
gateway-1-wan
```

Every DHCP-served interface receives the role-qualified
`<service>-<replica>-<role>` alias. The primary interface also receives
`<service>-<replica>`; the first replica additionally receives the bare service
name. The eligible `defaultRoute` interface is primary, falling back to the
first DHCP-served interface in manifest order.

ISC DHCP commit and renewal events atomically publish the active lease through
BNG dnsmasq. Release and expiry events remove only a matching MAC/address pair,
so a delayed event cannot delete a newer lease. Unknown client MAC addresses
may receive leases but are not published in deployment DNS.

Management-peer and DHCP records have separate runtime files. Refreshing
Aardvark-derived management records therefore cannot remove active device
leases, and DHCP callbacks cannot remove management aliases.

When a singleton `webconfig` service and BNG share a managed `mgmt` network,
BNG also resolves `webconfig` and `webconfig.dns.podman` to WebConfig's first
instance alias. Gateway's bundled `webcfg` client uses this route for its
development-only MessagePack download. No WebConfig alias is published across
different deployment management networks.

## Canonical interface environment contract

For each interface role, the renderer emits a canonical environment contract that
curated compose files consume (the role is upper-cased with `-` replaced by `_`,
e.g. `lan-p1` -> `LAN_P1`):

```
IFACE_<ROLE>_NETWORK
IFACE_<ROLE>_DEVICE
IFACE_<ROLE>_MAC
IFACE_<ROLE>_IPV4
IFACE_<ROLE>_IPV6
IFACE_<ROLE>_GATEWAY4
IFACE_<ROLE>_GATEWAY6
```

plus the deployment-level `DEPLOYMENT_NAME`, `SERVICE_NAME`, and `IMAGE`. Curated
`services/{bng,gateway,webpa}/compose.yaml` read these variables; `generic-container`
services have their compose document generated from the manifest.

## Privileges

Host bridge and firewall setup require root privileges. On macOS, `vcpe`
delegates these to the Podman machine Linux host automatically.

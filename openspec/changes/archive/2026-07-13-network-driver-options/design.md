## Context

Four code layers participate in network provisioning today: the manifest schema, the planner (manifest → plan), the orchestrator (plan → Podman calls), and the Podman adapter. The `EnsureNetwork` interface currently has five positional parameters (`name, subnet, hostGateway, podmanDNS string`) which is already straining — adding driver, options, and IPAM driver would make it unworkable. Switching to a `NetworkSpec` struct is a prerequisite.

The `hostnet.Intent` struct only carries `RequiresNAT` and `RequiresFirewall` bools — no driver awareness. The planner already controls whether these bools are set (it forces them for gateway LAN networks), so the fix is simply to not set them for non-bridge drivers at intent-build time in the orchestrator. No hostnet changes needed.

## Goals / Non-Goals

**Goals:**
- `manifest.Network` accepts `driver`, `driverOptions`, `ipamDriver`.
- `podman network create` passes `--driver`, `-o key=val` (one per entry), and `--ipam-driver` when set.
- `EnsureNetwork` takes a `NetworkSpec` struct.
- Bridge behavior (no `driver` field) is byte-for-byte identical to today.
- Validation rejects `nat/firewall` on non-bridge and requires `parent` for macvlan/ipvlan.

**Non-Goals:**
- IPAM integration for non-bridge drivers — vcpe assigns from the manifest pool regardless of driver.
- `ipamDriver: dhcp` container-IP discovery — containers still get pool-assigned static IPs.
- Changing `hostnet.Intent` struct.
- Supporting drivers other than the ones Podman itself supports — no vcpe-side driver whitelist beyond requiring `parent` for the known macvlan/ipvlan case.

## Decisions

**`NetworkSpec` struct replaces positional parameters in `EnsureNetwork`**
Converts the current five-arg function into a clean struct. All call sites are in the orchestrator (one loop) and the test doubles. The struct carries: `Name`, `Subnet`, `HostGateway`, `DNS`, `Driver`, `DriverOptions map[string]string`, `IPAMDriver`.

**Driver defaults to `bridge` when empty; no explicit default stored**
`""` means "let Podman use its default" (which is `bridge`). The adapter only adds `--driver` when the field is non-empty, preserving exact backward compatibility.

**`-o key=val` for each entry in `DriverOptions`**
Podman accepts multiple `-o` flags. Iterating over the map and appending one `-o key=val` per entry is portable and matches `podman network create` CLI conventions.

**Planner skips `HostBridgeGateway`/`PodmanDNS` for non-bridge drivers**
These fields are bridge-specific (they control the Podman host bridge IP and DNS injection). For macvlan/ipvlan there is no host bridge, so setting them is meaningless and potentially confusing.

**Orchestrator forces `RequiresNAT = false`, `RequiresFirewall = false` for non-bridge networks**
macvlan/ipvlan traffic bypasses the host network stack; iptables NAT/firewall rules on the host do not intercept it. Silently ignoring `nat: true` is better than applying rules that have no effect, but validation already rejects that combination.

**`ipamDriver` is a passthrough with no vcpe-side IPAM integration**
Stored in the struct, passed to `--ipam-driver` if non-empty. vcpe's IPAM pool assignment still runs for the container regardless (it assigns from the manifest pool). Expert users who set `ipamDriver: dhcp` and skip the `pool` field accept responsibility for the IPAM contract gap.

## Risks / Trade-offs

**Risk: map iteration order in `DriverOptions` makes args non-deterministic**
→ Sort keys before building `-o` args. Deterministic arg order is required for the adapter's unit tests and avoids spurious "network already exists" retries when arg order differs.

**Risk: `podman network create` idempotency with driver options**
→ Existing code returns early if `podman network exists` succeeds. Driver options are only applied at creation time; updating them requires destroy+recreate (documented as disruptive, same as CIDR changes).

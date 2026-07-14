## 1. Gateway Container Image

- [x] 1.1 Remove the `ethernets` section (and `eth0: dhcp4: yes`) from `services/gateway/container/50-cloud-init.yaml` — leave only the version/renderer header so netplan generates no `.network` file for eth0, preventing networkd from unslaving it from brlan0
- [x] 1.2 Run `vcpe build --manifest manifests/example.yaml --backend docker` (or Podman equivalent) to rebuild the gateway image
- [x] 1.3 Push the updated gateway image: `vcpe push --manifest manifests/example.yaml --backend docker`

## 2. IPAM — Skip for ipamDriver: none Networks

- [x] 2.1 In `CheckConflicts` (`internal/ipam/store.go`): skip `networkCIDRs(n)` for networks where `n.IPAMDriver == "none"` — no CIDR conflict check needed since Podman doesn't manage these subnets
- [x] 2.2 In `AllocateInterfaces` (`internal/ipam/store.go`): when `net.IPAMDriver == "none"`, skip pool allocation for that interface (leave `iface.IPv4` empty if not explicitly set)
- [x] 2.3 Add unit tests for both: network with `ipamDriver: none` is not conflict-checked; interface on `ipamDriver: none` network gets no pool allocation

## 3. Renderers — Omit ipv4_address for ipamDriver: none

- [x] 3.1 In `internal/types/bng/bng.go`: when building the service network entry, check if the interface's network has `IPAMDriver == "none"`; if so, omit `ipv4_address` from the network map (keep `mac_address`)
- [x] 3.2 In `internal/types/gateway/gateway.go`: same — omit `ipv4_address` for `ipamDriver: none` networks
- [x] 3.3 In `internal/types/genericcontainer/genericcontainer.go`: same — omit `ipv4_address` for `ipamDriver: none` networks
- [x] 3.4 Update golden render tests for bng, gateway, and generic-container to cover the `ipamDriver: none` case (no `ipv4_address` in network entry)

## 4. Update Example Manifests

- [x] 4.1 In `manifests/example.yaml`: add `ipamDriver: none` to `wan`, `cm`, `lan-p1`, `lan-p2`, `lan-p3`, `lan-p4` networks; leave `mgmt` unchanged
- [x] 4.2 In `manifests/example-macvlan.yaml`: add `ipamDriver: none` to `wan` (already has `driver: macvlan`; also add to `cm`, `lan-*` for consistency)

## 5. Spec Sync

- [x] 5.1 Apply MODIFIED `desired-state-manifests` spec to `openspec/specs/desired-state-manifests/spec.md`
- [x] 5.2 Apply MODIFIED `podman-reconciliation-engine` spec to `openspec/specs/podman-reconciliation-engine/spec.md`

## 6. Verification

- [x] 6.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 6.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 6.3 Run `vcpe up --manifest manifests/example.yaml` with the new image
- [x] 6.4 Verify `podman exec example-client_client_1 ip addr show eth0` shows a `192.168.10.x` DHCP lease
- [x] 6.5 Verify `podman exec example-gateway bridge link` shows eth0 in `forwarding` state on brlan0 after systemd starts

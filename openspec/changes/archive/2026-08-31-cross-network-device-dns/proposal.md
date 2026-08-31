## Why

Management-network services such as WebPA cannot resolve Gateway or XB10 instances that receive addresses from BNG DHCP on WAN or CM networks. The BNG already sees those leases and can route management traffic to them, but the lease identities are not published through a DNS authority used by management services.

## What Changes

- Publish active BNG DHCPv4 leases as deployment-local DNS records with stable service-instance names.
- Remove or update those records when leases expire, are released, or move to a new address.
- Make management-network services resolve BNG-published device names while preserving Podman/Aardvark resolution for management peers and normal upstream DNS resolution.
- Route WebPA traffic for BNG-connected access networks through the BNG management address so resolved device names are usable for callbacks and diagnostics.
- Define deterministic behavior for replicas, multiple DHCP interfaces, client-supplied hostnames, and naming conflicts.
- Add renderer, lease-lifecycle, and live cross-network resolution coverage.

## Capabilities

### New Capabilities

- `cross-network-device-dns`: Defines how DHCP-attached service instances are named, published by BNG DNS, resolved from management networks, and removed across lease lifecycle events.

### Modified Capabilities

None.

## Impact

- BNG rendering and runtime assets under `controlplane/internal/types/bng` and `services/bng`.
- Resolved service/network metadata used to generate DHCP identity mappings and management-side resolver configuration.
- Management-network service startup or Compose DNS configuration, including WebPA and other services that need cross-network device lookup.
- Podman, BNG dnsmasq, and ISC DHCP integration tests plus deployment smoke coverage.
- No manifest schema or public CLI change is expected.
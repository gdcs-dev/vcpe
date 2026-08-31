## Context

Podman Aardvark DNS only knows containers attached to the queried Podman network. WebPA and other management services therefore resolve management peers but cannot resolve Gateway or XB10 instances attached only to BNG WAN or CM networks.

BNG already provides the missing authority boundary. ISC DHCP serves WAN and CM leases, dnsmasq listens on BNG loopback and interface addresses, and BNG routes management traffic to leased access addresses. The image also contains a lease-notify script and generated mapping artifacts, but rendered `dhcpd.conf` does not invoke the script and the MAC mapping is empty. The current script and BNG entrypoint also share one dynamic hosts file, so a DHCP refresh would overwrite management-peer records seeded from Aardvark.

The apply pipeline runs `ipam.AllocateInterfaces` before rendering. Consequently, the renderer and shared Compose builder can see BNG's allocated management address and every planned interface MAC without adding manifest fields or runtime discovery.

## Goals / Non-Goals

**Goals:**

- Publish the current DHCPv4 address of each planned Gateway, XB10, or other service interface on a BNG access segment.
- Provide stable deployment-local names for service instances, replicas, and individual access roles.
- Let non-BNG management services query those names while retaining management-peer and external DNS resolution.
- Let WebPA connect to resolved access addresses through BNG for callbacks and diagnostics.
- Keep lease updates atomic and prevent release, expiry, or concurrent callbacks from leaving stale records.
- Reuse allocated plan identities and the existing BNG DHCP/dnsmasq processes.

**Non-Goals:**

- Expose deployment DNS to the host or outside the deployment's networks.
- Publish arbitrary, undeclared DHCP clients or trust client-supplied hostnames as deployment identities.
- Add a manifest DNS schema or CLI surface.
- Replace ISC DHCP with dnsmasq DHCP.
- Make DHCP leases survive BNG container replacement; this change preserves records across process restart where runtime state remains and republishes them on subsequent lease events.
- Add general firewall policy beyond the routes required for WebPA to reach BNG-connected access networks.

## Decisions

### 1. BNG dnsmasq is the deployment DNS bridge

The shared Compose service builder will locate the BNG instance attached to the same managed `mgmt` network as a non-BNG service. When that allocated BNG address exists, the rendered service will receive a DNS list containing the BNG management address first and the Podman network gateway/Aardvark address second.

BNG itself keeps its original Aardvark resolver. At startup it continues capturing that resolver as dnsmasq's upstream, so names not owned by BNG, including management aliases and external domains, are forwarded to Aardvark. The second resolver gives management workloads a fallback when BNG DNS is temporarily unavailable.

This is preferred over setting Podman's network-level `--dns`: a network-wide override would also affect BNG and can create a BNG-to-itself forwarding loop. Implementing the rule in every renderer was rejected because the shared Compose builder already owns the common service block and covers built-in and generic services consistently.

If no BNG shares the service's management network, rendering leaves DNS unchanged. A BNG with no allocated management address is likewise not selected.

### 2. Planned MAC identity controls published names

The BNG renderer will generate a DHCP identity map for each non-BNG service interface whose role is configured as a BNG DHCPv4 access segment. The deterministic planned MAC is the lookup key; a DHCP client's option 12 hostname cannot override this mapping.

Each eligible interface receives a role-qualified alias:

- `<service>-<replica>-<role>`, for example `xb10-1-wan`.

Each instance also receives `<service>-<replica>` on one primary access interface. The primary is the eligible interface marked `defaultRoute`; when none is marked, it is the first eligible interface in manifest order. Replica one additionally owns the bare `<service>` alias, matching the established first-instance alias convention. Thus `xb10`, `xb10-1`, and `xb10-1-wan` can identify the first XB10 WAN lease, while `xb10-1-cm` identifies a separate CM lease when one exists.

The renderer will reject duplicate generated aliases instead of silently choosing an address. Unknown MACs are logged but not published. This prevents an undeclared client from claiming names such as `webpa` or `gateway` through a DHCP hostname.

### 3. ISC DHCP callbacks drive the DNS lease lifecycle

The generated DHCPv4 configuration will install global commit, release, and expiry callbacks that invoke the BNG lease-notify script with event, leased address, and client MAC. A commit looks up all planned aliases for the MAC and replaces prior records for that identity. Release and expiry remove only records matching the event's MAC and address, so a delayed event for an old lease cannot remove a newer lease.

The script will serialize updates with a lock, update its state and hosts file through temporary-file rename, and signal dnsmasq only after a successful replacement. Multiple aliases may point to the same active lease address. State that remains in the same container is reused after process restart; subsequent commits and renewals reconcile it normally.

Client-supplied hostnames may be retained in diagnostic logs but are not DNS inputs. This is preferred over hostname-first publication because DHCP option 12 is neither stable nor trusted, and the live XB10 hostname differs from the deployment service identity.

### 4. Management and DHCP records have separate owners

BNG dnsmasq will read distinct additional hosts files:

- a management-peer file written by the BNG entrypoint after resolving planned management aliases through Aardvark;
- a DHCP lease file written only by the lease-notify script.

Separating the files prevents a lease callback from deleting WebPA, Argus, event-sink, or other management records. dnsmasq CNAMEs for virtual WebPA names continue to target management aliases. Both files are initialized before dnsmasq starts, and either owner may atomically refresh its file followed by SIGHUP.

### 5. Validation spans rendered configuration and a live namespace path

Unit tests will lock down generated MAC mappings, alias selection, callback blocks, file separation, and shared Compose DNS ordering. Script-level tests will exercise commit, address move, delayed release, expiry, unknown MAC, duplicate alias, and concurrent updates with a fake dnsmasq signal target.

A Podman smoke test will start BNG, WebPA, and at least one DHCP-attached Gateway or XB10; wait for a lease; resolve the bare, instance, and role-qualified names from WebPA; verify the answer equals the active lease; and confirm an Aardvark management alias and an external name still resolve.

### 6. WebPA routes access traffic through BNG

The WebPA renderer will locate the BNG instance attached to the same managed `mgmt` network and emit its allocated management IPv4 address plus the IPv4 CIDRs of its other attached deployment networks. WebPA's entrypoint will install one route per emitted CIDR through that BNG address before starting Caduceus and the other WebPA services.

This is required because a Podman-managed network gives WebPA a default route through the Podman bridge gateway. That path may resolve and pass ICMP to a DHCP-attached device while rejecting or dropping callback TCP traffic. The explicit routes keep management-to-access traffic on the deployment's intended BNG path. If no eligible BNG or routed CIDR exists, rendering and startup remain unchanged.

## Risks / Trade-offs

- **[Risk] BNG becomes the first DNS hop for management workloads.** A BNG startup failure can delay DNS. **Mitigation:** retain Aardvark as the second resolver and preserve existing dependency ordering where declared.
- **[Risk] Resolver libraries do not retry a secondary server after an authoritative negative answer.** **Mitigation:** BNG forwards unknown names to its captured Aardvark upstream rather than returning a local negative answer for names outside its data.
- **[Risk] A rendered route becomes invalid if BNG topology changes without recreating WebPA.** **Mitigation:** routes derive from the applied plan and are installed on every WebPA container start; topology changes already require reconciliation.
- **[Risk] A late release can race a renewed lease.** **Mitigation:** key state by MAC, alias, and address; serialize changes; remove only matching old records.
- **[Risk] Generated aliases can collide with unusual service names.** **Mitigation:** detect collisions during rendering and fail before runtime mutation.
- **[Trade-off] Unknown DHCP clients are not discoverable by hostname.** This intentionally favors deterministic deployment identity and prevents DHCP hostname spoofing.
- **[Trade-off] Fresh BNG container replacement loses in-container DHCP/DNS lease state until clients renew.** Persisting ISC leases across replacement is a broader lifecycle change and remains out of scope.

## Migration Plan

1. Add renderer and script tests before changing runtime behavior.
2. Render identity mappings, callback configuration, separate hosts files, and management-service DNS lists.
3. Rebuild the BNG image and apply an existing Gateway and XB10 manifest; no manifest edits are required.
4. Run focused Go tests, script tests, and the Podman smoke scenario.
5. Roll back the control-plane binary and BNG image together if necessary; reapplying the prior version restores Aardvark-only management resolution and removes the new callback artifacts.

## Open Questions

None. Naming, trust, resolver ordering, and lease-state scope are defined above.
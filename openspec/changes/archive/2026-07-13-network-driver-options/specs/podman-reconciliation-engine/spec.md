## ADDED Requirements

### Requirement: EnsureNetwork accepts NetworkSpec struct
The `networkProvisioner` interface and `podman.EnsureNetwork` implementation SHALL accept a `NetworkSpec` struct instead of positional string parameters. The struct SHALL carry `Name`, `Subnet`, `HostGateway`, `DNS`, `Driver`, `DriverOptions`, and `IPAMDriver`. When `Driver` is non-empty, `podman network create` SHALL include `--driver <driver>`. Each entry in `DriverOptions` SHALL be passed as a separate `-o key=val` flag with keys in sorted order. When `IPAMDriver` is non-empty, `--ipam-driver <driver>` SHALL be included. When `Driver` is empty the behavior SHALL be identical to the previous implementation.

#### Scenario: macvlan network created with parent
- **WHEN** a deployment with a macvlan network is applied
- **THEN** the reconciler invokes `podman network create --driver macvlan -o parent=eth0 [--subnet ...] <name>`

#### Scenario: bridge network creation is unchanged
- **WHEN** a deployment with a standard bridge network (no driver field) is applied
- **THEN** `podman network create` is called without `--driver`, identical to existing behavior

#### Scenario: NAT/firewall intents not generated for non-bridge networks
- **WHEN** a deployment containing a macvlan network is applied
- **THEN** the hostnet phase does not attempt to configure NAT or firewall rules for that network

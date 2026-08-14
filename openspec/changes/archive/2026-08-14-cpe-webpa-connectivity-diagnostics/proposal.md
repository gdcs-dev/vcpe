## Why

Application developers can see that a CPE-to-WebPA workflow is unhealthy, but they must inspect multiple containers and interpret Parodus and XMiDT logs to find the broken boundary. vCPE should use its knowledge of deployment topology and service-specific protocols to show the expected connection path, identify the first confirmed failure, and provide bounded actionable evidence.

## What Changes

- Add a `vcpe diagnose --name <deployment> --from <service> --to webpa --client-service <name>` command for diagnosing a deployed CPE application's path through local Parodus to Talaria/WebPA.
- Present an expected-versus-observed ASCII connection graph in human output, with passed, failed, and skipped stages and one first causal failure.
- Provide stable JSON graph output containing nodes, edges, observations, evidence, and remediation identifiers for automation and future visual clients.
- Diagnose application-to-Parodus connectivity, Talaria name resolution and transport reachability, Talaria authentication, and authoritative device registration.
- Query Parodus's existing `service-status/<service>` client list through a direct Scytale WRP Retrieve so receive-enabled application registration is verified without changing Parodus or libparodus.
- Allow callers to select the receive-enabled client service through a bounded stable identifier while the source retains ownership of the WRP endpoint, credentials, device identity, source, and destination prefix.
- Derive expected topology and target configuration from persisted deployment state and service-type knowledge, then collect bounded active and passive observations without parsing unbounded logs.
- Extend the existing loopback-published per-instance HTTP service with diagnostic capability discovery and a versioned CPE-to-WebPA diagnostic endpoint.
- Run active probes in the source workload's network namespace behind that HTTP endpoint so DNS and routing observations remain source-accurate without control-plane container exec.
- Redact secrets and bound all diagnostic evidence, messages, probe timeouts, and output cardinality.
- Keep routine `vcpe status` passive; diagnostics are explicitly requested active operations, but both communicate exclusively through persisted loopback HTTP endpoints.
- Exclude webhook registration, callback delivery, end-to-end event correlation, general-purpose log analysis, and VS Code diagnostic visualization. Those can build on the graph contract in follow-up changes.

## Capabilities

### New Capabilities
- `cpe-webpa-connectivity-diagnostics`: Expected-path modeling, bounded diagnostic execution, first-failure analysis, visual human output, and structured JSON for the CPE application to Parodus to WebPA path.

### Modified Capabilities
- `local-control-plane-cli`: Add the deployment-targeted `diagnose` command and its human and JSON output contract to the Go-owned operator surface.

## Impact

- Adds CLI parsing, structured help, command dispatch, human rendering, and JSON output in `controlplane/internal/app`.
- Adds a diagnostic domain model and orchestrator in the control plane, including a stable graph schema and bounded probe execution.
- Extends the common per-instance HTTP service with `GET /diagnostics` capability discovery and `POST /diagnostics/cpe-webpa` while preserving the existing `GET /health` schema and behavior.
- Extends service-type-owned behavior for Gateway/XB10 and WebPA so protocol-specific expectations and workload-local observations are not hardcoded into generic CLI formatting.
- Adds `wrp-go` decoding for bounded Scytale msgpack responses; credentials and the configured Parodus service name remain source-local and are never supplied by a diagnostic request.
- Reuses each source instance's persisted loopback health endpoint and requires no Podman, Docker, Compose, container-name discovery, or container-exec calls from `vcpe`.
- Adds focused unit, HTTP contract, and deployed integration tests for success and representative DNS, transport, authentication, and registration failures.
- Does not change the manifest schema, common health response schema, deployment lifecycle, or visual editor in this change.
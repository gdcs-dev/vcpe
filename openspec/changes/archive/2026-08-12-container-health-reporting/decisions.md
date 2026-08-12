## Decisions

### Decision: Control-plane health transport
[BREAKING]
Recommendation: Query typed container endpoints over loopback-published HTTP ports.
Decision: Proceed with the recommended approach.
Rationale: The operator needs live health without making Podman calls, and HTTP endpoints remain usable when Podman runs through a macOS VM.

Q: How should `vcpe` obtain service health without Podman calls?
A: Query container-owned health endpoints over HTTP.

---

### Decision: Health evaluation ownership
[BREAKING]
Recommendation: Put type-specific checks in the service/container and make the control plane a collector only.
Decision: Proceed with the recommended approach.
Rationale: The service owns the semantics of its readiness and can provide detailed, stable diagnostics.

Q: Where should unique health checks execute?
A: In each container behind a common `/health` contract.

---

### Decision: Gateway WebPA readiness condition
[BREAKING]
Recommendation: Require both reachability and a recent authoritative WebPA registration observation.
Decision: Proceed with the recommended approach.
Rationale: Talaria reachability alone cannot demonstrate an active Gateway/Parodus connection.

Q: What must Gateway report for its WebPA condition?
A: Registered and connected, not merely able to reach WebPA.

---

### Decision: Generic container health policy
[BREAKING]
Recommendation: Require explicit health configuration for generic containers and report an absent configuration as `not-configured`.
Decision: Proceed with the recommended approach.
Rationale: A generic process cannot be honestly declared healthy using only container liveness.

Q: How should untyped generic workloads participate?
A: They opt into an explicit health probe; otherwise the CLI reports that health is not configured.
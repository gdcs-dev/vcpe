## Context

Architecture and all implementation decisions are captured in `architecture.md` and `decisions.md` — this document does not repeat them. Design focus here is on implementation-level details those artifacts don't address: build integration, package layout, event fixture content, and operational concerns.

**Current state**: The vCPE gateway container runs parodus with a live WebSocket connection to the XMiDT cloud stack (Talaria/Caduceus/Argus). No tooling exists to generate test events through this pipeline or consume them on the cloud side.

**Constraints**: The control plane is mid-rewrite (`manifest-driven-redesign`). The event-sink must not be registered as a vCPE service type. Both new services are purely additive.

## Goals / Non-Goals

**Goals:**
- Deliver a gateway-side C daemon that emits synthetic AppArmor events over the live parodus connection on a configurable timer and on-demand via RBUS
- Deliver a cloud-side Go service that receives those events via a durable Argus-backed webhook and logs them
- Make the cloud-side receiver generic enough to capture any event type by changing `WEBHOOK_EVENTS_REGEX`

**Non-Goals:**
- Real AppArmor kernel integration (no `/proc/1/attr` or audit subsystem reads)
- Production security monitoring or alerting
- Persistence of received events beyond structured stdout logs
- Registering `event-sink` as a vCPE manifest service type (deferred)
- Multi-instance event-sink (single webhook registration per deploy)

## Decisions

### C source layout

```
services/gateway/src/apparmor-simulator/
    CMakeLists.txt
    apparmor-simulator.c       # main: init, timer loop, RBUS method, send
    events.h                   # struct AppArmorEvent + fixture table declaration
    events.c                   # fixture array definition
```

Single compilation unit is acceptable for this scope. `events.h/c` are separated so the fixture table can be reviewed and extended without touching the main loop.

**CMake link order**: `wrp-c` → `libparodus` (nanomsg) → `rbus` → `cjson` → `pthread`. The order matters because libparodus links nanomsg symbols; linking `wrp-c` first ensures its symbols are resolved before libparodus pulls in nanomsg.

### AppArmor event fixture table

Twelve hardcoded events covering three gateway-resident processes and two severities. Simulator cycles through them in order (index % 12) for deterministic output:

| # | profile | operation | name | apparmor | severity |
|---|---------|-----------|------|----------|----------|
| 0 | `/usr/sbin/dnsmasq` | `open` | `/etc/shadow` | DENIED | denied |
| 1 | `/usr/sbin/dnsmasq` | `open` | `/proc/net/fib_trie` | AUDIT | audit |
| 2 | `/usr/bin/parodus` | `exec` | `/usr/bin/curl` | DENIED | denied |
| 3 | `/usr/bin/parodus` | `open` | `/etc/ssl/private/key.pem` | DENIED | denied |
| 4 | `/usr/sbin/dnsmasq` | `connect` | `socket:[family=2,type=1]` | DENIED | denied |
| 5 | `/usr/bin/rbus_elements` | `open` | `/proc/sys/kernel/hostname` | AUDIT | audit |
| 6 | `/usr/sbin/dnsmasq` | `open` | `/var/run/dnsmasq.pid` | ALLOWED | allowed |
| 7 | `/usr/bin/parodus` | `open` | `/tmp/parodus_cfg.json` | AUDIT | audit |
| 8 | `/usr/sbin/telemetry2_0` | `open` | `/etc/telemetry/profile.json` | DENIED | denied |
| 9 | `/usr/sbin/telemetry2_0` | `exec` | `/usr/bin/curl` | DENIED | denied |
| 10 | `/usr/bin/rbus_elements` | `open` | `/sys/firmware/devicetree/base/serial-number` | AUDIT | audit |
| 11 | `/usr/bin/parodus` | `open` | `/etc/ssl/certs/ca-certificates.crt` | ALLOWED | allowed |

PIDs are simulated with `(getpid() + index) % 65535`. FSUIDs use realistic gateway service UIDs (33 for www-data services, 0 for root-owned processes).

### event-sink Go package layout

```
services/event-sink/
    go.mod                             # module github.com/gdcs-dev/vcpe/event-sink
    go.sum
    cmd/
        event-sink/
            main.go                    # flag parsing, startup, shutdown
    internal/
        registration/
            registration.go            # ancla InternalWebhook + svc.Add + 6h refresh ticker
            registration_test.go
        handler/
            handler.go                 # HTTP webhook handler + HMAC-SHA1 validation + wrphttp decode
            handler_test.go
    Containerfile                      # multi-stage: golang builder → debian:bookworm-slim runtime
    container/entrypoint.sh            # dhclient on eth0, then exec event-sink
    compose.yaml                       # rendered compose.env (vcpe apply), networks: mgmt
```

> **As-built note:** registration uses `ancla` v0.3.12 (`NewService`+`Add`), not
> raw Argus PUTs; HMAC is SHA1; the WRP message is reconstructed from HTTP
> headers. See decisions.md § As-Built Corrections.

`internal/` enforces that registration and handler packages are not importable outside the module, which is correct for an application (not a library).

### Containerfile placement for apparmor-simulator build block

The new `COPY` + CMake block in Stage 1 must be placed **after** the `libparodus` build block and **after** the `rbus` build block, because `CMakeLists.txt` uses `find_package` / pkg-config to locate those libraries. The `cmake` call must pass `-DCMAKE_C_FLAGS` with the same `-I$INSTALL_PREFIX/include/...` flags used by parodus and parodus2rbus.

Placement in the Containerfile: after the `syscfg` build block (last existing block), before Stage 2.

### Startup ordering in gateway container

`apparmor-simulator.service` unit:
```ini
[Unit]
After=rbus.service parodus.service
```

No `Requires=` on either. The retry loop in the daemon handles both rbus and parodus being transiently unavailable. `After=` is belt-and-suspenders for clean startup ordering when all services start together.

### event-sink startup sequence

```
main()
  1. Parse flags / env vars (ARGUS_URL, ARGUS_BASIC_AUTH, WEBHOOK_URL, WEBHOOK_EVENTS_REGEX,
     WEBHOOK_DEVICE_MATCHER, WEBHOOK_SECRET)
  2. Start http.Server in goroutine (so /health responds immediately)
  3. registration.Register(ctx, config) — blocking retry loop until first Argus PUT succeeds
  4. Start registration.RefreshLoop(ctx, config) goroutine
  5. Block on os.Signal (SIGTERM/SIGINT) → graceful shutdown
```

Step 3 blocks intentionally: the event-sink is not useful until registered. The HTTP server being up in step 2 allows liveness probes to pass while registration is retrying.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| `erouter0` not available when apparmor-simulator starts | Error path exits; `Restart=always` retries every 15s until interface comes up |
| libparodus connect fails on first attempt (parodus not yet connected to cloud) | `libparodus_init` retry loop: exponential backoff up to 60s |
| Argus TTL expiry if event-sink is down > 12h | `ancla` `svc.Add()` on restart re-establishes the item idempotently; 6h refresh ticker keeps it alive |
| `WEBHOOK_EVENTS_REGEX` typo results in no events delivered | event-sink logs the compiled regex on startup at INFO level; easy to spot |
| ancla/argus API changes across versions | Pin `ancla v0.3.12` + `argus v0.9.10` to match the deployed Caduceus; do not auto-upgrade (newer ancla uses an incompatible ManifestV1/V2 schema) |
| apparmor-simulator cycles through fixture index; wrap-around at 12 events | Intentional; deterministic output is a feature for testing |
| event-sink compose started without `--env-file` | `${IFACE_MGMT_NETWORK}` expands to empty string; container fails to attach to mgmt network with clear error |

## Why

The vCPE dev/test harness has no way to exercise the full XMiDT event pipeline end-to-end. Developers cannot validate that the gateway container's parodus connection is working, that events route correctly through Talaria and Caduceus, or that cloud-side consumers receive and decode WRP events correctly — without deploying real firmware or writing one-off test scripts. This change adds dedicated tooling to close that gap.

## What Changes

- **New**: `apparmor-simulator` C daemon added to the gateway container — generates realistic AppArmor audit events on a configurable timer and sends them via `libparodus` → parodus → XMiDT as `WRP_MSG_TYPE__EVENT` messages. Also exposes an on-demand RBUS method `Device.AppArmor.SimulateEvent()`.
- **New**: `event-sink` Go service — registers a configurable webhook with Argus (Argus-backed, durable) and logs all matching WRP events delivered by Caduceus. Configurable via `WEBHOOK_EVENTS_REGEX` and `WEBHOOK_DEVICE_MATCHER` env vars; reusable for any event type, not just AppArmor.
- **New**: `services/gateway/src/apparmor-simulator/` — in-tree C source, CMake build, integrated into the gateway Containerfile builder stage.
- **New**: `services/event-sink/` — standalone Go service with its own compose.yaml, Containerfile, and Go module.
- **Modified**: `services/gateway/Containerfile` — adds `COPY` + CMake build block and runtime systemd unit for `apparmor-simulator`.

## Capabilities

### New Capabilities

- `apparmor-event-simulation`: A device-side C service that generates synthetic AppArmor audit events over the XMiDT WRP pipeline. Events carry realistic kernel audit fields (`operation`, `profile`, `name`, `pid`, `comm`, `requested_mask`, `denied_mask`) and a `simulated: true` marker. Event destination format: `event:apparmor/{severity}/mac:{device}`.
- `xmidt-event-sink`: A reusable cloud-side webhook consumer that registers with Caduceus via Argus-backed webhook registration and logs received WRP events as structured JSON. Configurable events regex and device matcher make it applicable to any event type in the XMiDT pipeline.

### Modified Capabilities

<!-- No existing spec-level requirements are changing. The gateway container and webpa/XMiDT stack behavior is unchanged — new services are purely additive. -->

## Impact

- **`services/gateway/Containerfile`**: New COPY + CMake build block in Stage 1; new systemd unit COPY + `systemctl enable` in Stage 2.
- **`services/gateway/src/apparmor-simulator/`**: New in-tree C source directory (CMakeLists.txt, apparmor-simulator.c).
- **`services/gateway/container/apparmor-simulator.service`**: New systemd unit file.
- **`services/event-sink/`**: New service directory — `go.mod`, `cmd/event-sink/main.go`, `Containerfile`, `compose.yaml`.
- **Dependencies added**: `github.com/xmidt-org/ancla/chrysom` (Argus HTTP client) in the event-sink Go module.
- **Runtime**: apparmor-simulator reads `/sys/class/net/erouter0/address` for device MAC at startup. event-sink must be started with `--env-file ../webpa/compose.env` to inherit `IFACE_MGMT_NETWORK`.
- **No breaking changes** to existing services, manifests, or the control plane.

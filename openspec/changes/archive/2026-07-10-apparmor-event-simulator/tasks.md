## 1. apparmor-simulator: C source scaffolding

- [x] 1.1 Create `services/gateway/src/apparmor-simulator/` directory with `CMakeLists.txt` linking `wrp-c`, `libparodus`, `rbus`, `cjson`, `pthread` in correct order
- [x] 1.2 Create `services/gateway/src/apparmor-simulator/events.h` defining `AppArmorEvent` struct and declaring the fixture array
- [x] 1.3 Create `services/gateway/src/apparmor-simulator/events.c` with the 12-entry fixture table (see design.md fixture table)
- [x] 1.4 Create `services/gateway/src/apparmor-simulator/apparmor-simulator.c` main entry point: reads `APPARMOR_SIM_INTERVAL_SEC`, reads MAC from `/sys/class/net/erouter0/address`, calls `rbus_open` + `rbus_regDataElements`, calls `libparodus_init` with retry loop, arms `timerfd`

## 2. apparmor-simulator: core logic

- [x] 2.1 Implement `read_mac_address()`: open `/sys/class/net/erouter0/address`, read, strip colons, store in global buffer; exit with cimplog ERROR if open fails
- [x] 2.2 Implement `emit_apparmor_event(int index)`: build WRP EVENT with correct `source` (`mac:{mac}/apparmor-simulator`), `dest` (`event:apparmor/{severity}/mac:{mac}`), JSON payload from fixture; call `libparodus_send`; log WARN on send failure
- [x] 2.3 Implement `timerfd` event loop: create `timerfd_create(CLOCK_MONOTONIC)`, set interval from env var, `epoll_wait` loop calling `emit_apparmor_event(counter++ % 12)` on expiry
- [x] 2.4 Implement RBUS method handler `apparmor_simulate_method()`: calls `emit_apparmor_event` with current counter value, increments counter, returns `RBUS_ERROR_SUCCESS`
- [x] 2.5 Implement libparodus init retry loop: exponential backoff starting at 1s, capped at 60s, log ERROR on each failure and INFO on success
- [x] 2.6 Add `cimplog`-based logging: INFO for start/register/emit, WARN for send failure/RBUS fail, ERROR for MAC read fail/fatal errors

## 3. apparmor-simulator: gateway container integration

- [x] 3.1 Create `services/gateway/container/apparmor-simulator.service` systemd unit with `After=rbus.service parodus.service`, `Restart=always`, `RestartSec=15`, `EnvironmentFile` pointing to a drop-in for `APPARMOR_SIM_INTERVAL_SEC`
- [x] 3.2 Add builder stage block to `services/gateway/Containerfile`: `COPY src/apparmor-simulator /opt/git/apparmor-simulator` followed by `cmake`/`make`/`make install` with correct `-DCMAKE_C_FLAGS` include paths (matching parodus2rbus flags)
- [x] 3.3 Add runtime stage lines to `services/gateway/Containerfile`: `COPY container/apparmor-simulator.service /usr/lib/systemd/system/` and `RUN systemctl enable apparmor-simulator`
- [x] 3.4 Verify gateway image builds successfully with the new block (`podman build` or equivalent)

## 4. event-sink: Go module and package scaffolding

- [x] 4.1 Create `services/event-sink/go.mod` with module `github.com/gdcs-dev/vcpe/event-sink`, Go 1.24, dependencies: `github.com/xmidt-org/ancla/chrysom`, `golang.org/x/crypto` (for HMAC)
- [x] 4.2 Create `services/event-sink/internal/handler/handler.go`: `http.Handler` that validates `X-Webpa-Signature` HMAC-SHA256, decodes WRP event body, logs with `slog`; returns 401 on invalid HMAC, 400 on malformed body, 200 on success
- [x] 4.3 Create `services/event-sink/internal/handler/handler_test.go`: table-driven tests for valid HMAC, missing header, wrong secret, malformed body
- [x] 4.4 Create `services/event-sink/internal/registration/registration.go`: `Register(ctx, cfg)` blocking retry loop doing `chrysom` PUT to Argus `webhooks` bucket; `RefreshLoop(ctx, cfg)` goroutine with `time.NewTicker(6h)`
- [x] 4.5 Create `services/event-sink/internal/registration/registration_test.go`: tests using `httptest.Server` mocking Argus — successful registration, retry on 503, refresh tick
- [x] 4.6 Create `services/event-sink/cmd/event-sink/main.go`: parse env vars (`ARGUS_URL`, `ARGUS_BASIC_AUTH`, `WEBHOOK_URL`, `WEBHOOK_EVENTS_REGEX`, `WEBHOOK_DEVICE_MATCHER`, `WEBHOOK_SECRET`), validate regexes (fail-fast on invalid), start HTTP server (including `/health`), call `Register`, start `RefreshLoop`, block on SIGTERM

## 5. event-sink: container and compose

- [x] 5.1 Create `services/event-sink/Containerfile`: multi-stage `golang` builder → `debian:bookworm-slim` runtime (changed from alpine; needs iproute2/isc-dhcp-client for DHCP + hostname registration); copies `event-sink`, `entrypoint.sh`, `runtime-init-event-sink`; `EXPOSE 8080`; `ENTRYPOINT [runtime-init-event-sink]` + `CMD [entrypoint.sh]`
- [x] 5.2 Create `services/event-sink/compose.yaml`: service uses `${IMAGE}`/`${SERVICE_NAME}`/`${IFACE_MGMT_MAC}`/`${IFACE_MGMT_NETWORK}` from generated `compose.env`, env vars (`ARGUS_URL`, `ARGUS_BASIC_AUTH`, `WEBHOOK_URL`, `WEBHOOK_EVENTS_REGEX`, `WEBHOOK_DEVICE_MATCHER`, `WEBHOOK_SECRET`)
- [x] 5.3 Register `event-sink` as a vCPE service type (`controlplane/internal/types/eventsink/`, wired in `types.go`) so `vcpe apply` renders `compose.env`; add to orchestrator teardown list and `stage-runtime-init-binaries`
- [x] 5.4 Verify `go build ./...` and `go test ./...` pass in `services/event-sink/`
- [x] 5.5 Create `controlplane/cmd/runtime-init-event-sink/main.go` and stage the binary via `scripts/stage-runtime-init-binaries event-sink`
- [x] 5.6 Create `services/event-sink/container/entrypoint.sh`: dhclient on eth0 to acquire mgmt IP from BNG dnsmasq (registers hostname), then exec event-sink

## 6. End-to-end smoke test

- [x] 6.1 Start the full vCPE stack (`vcpe apply`) including the gateway and webpa services
- [x] 6.2 event-sink brought up by `vcpe apply` (registered service type)
- [x] 6.3 Confirm event-sink logs "webhook registered" within 30s of startup
- [x] 6.4 Confirm event-sink receives and logs AppArmor events (verified: full payload logged)
- [x] 6.5 Confirm on-demand RBUS method emission works (`Device.AppArmor.SimulateEvent`)
- [x] 6.6 Verify events regex filtering works (`apparmor/.*` — no `event:` prefix; Caduceus strips it)

## 8. Debugging fixes discovered during bring-up

- [x] 8.1 Fix include paths: `cimplog/cimplog.h`, `libparodus/libparodus.h`, `wrp-c.h`; keep `cimplog` as transitive link dep of libwrp-c
- [x] 8.2 Switch simulator logging from cimplog to syslog (`openlog`/`syslog`)
- [x] 8.3 Argus registration: use `ancla` v0.3.12 `InternalWebhook` + `svc.Add()` (not raw PUT / chrysom / ManifestV1)
- [x] 8.4 HMAC: switch handler from SHA256 to SHA1 (`sha1=` prefix); set `Config.Secret` in registration
- [x] 8.5 WRP decode: reconstruct from `X-Xmidt-*`/`X-Webpa-*` headers via `wrphttp.SetMessageFromHeaders`, payload in body
- [x] 8.6 Events regex: drop `event:` prefix (Caduceus strips scheme before matching)
- [x] 8.7 Orchestrator: remove stale `compose.yaml` from deployment state when a service migrates type (fixes `vcpe down` not tearing down event-sink)

## 7. Documentation

- [x] 7.1 Add `services/event-sink/README.md` documenting: purpose, env vars table, how to start, example log output
- [x] 7.2 Add comment block to `services/gateway/src/apparmor-simulator/apparmor-simulator.c` header describing build deps and env vars

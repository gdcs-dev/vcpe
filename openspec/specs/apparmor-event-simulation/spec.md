## Purpose
Simulate AppArmor security audit events on the gateway and deliver them to the XMiDT cloud pipeline via parodus, so the full device→cloud event path can be exercised without real firmware.

## Requirements

### Requirement: Simulator registers with parodus on startup
The apparmor-simulator daemon SHALL register with the local parodus process using `libparodus_init` on startup, with service name `apparmor-simulator`, parodus URL `tcp://127.0.0.1:6666`, and client URL `tcp://127.0.0.1:6670`. If registration fails, the daemon SHALL retry with exponential backoff (up to 60s ceiling) without exiting.

#### Scenario: Successful parodus registration
- **WHEN** apparmor-simulator starts and parodus is running
- **THEN** `libparodus_init` returns success and the daemon logs INFO "registered with parodus"

#### Scenario: Parodus unavailable at startup
- **WHEN** apparmor-simulator starts before parodus is ready
- **THEN** the daemon retries `libparodus_init` with exponential backoff and does not exit
- **THEN** once parodus becomes available, registration succeeds and the daemon begins emitting events

---

### Requirement: Timer-based event emission
The apparmor-simulator SHALL emit one `WRP_MSG_TYPE__EVENT` message per interval, where the interval is configured via the `APPARMOR_SIM_INTERVAL_SEC` environment variable (default: 30). If `APPARMOR_SIM_INTERVAL_SEC` is absent or unparseable, the daemon SHALL use the default value and log a WARNING.

#### Scenario: Timer fires at configured interval
- **WHEN** `APPARMOR_SIM_INTERVAL_SEC=10` is set and the daemon is running
- **THEN** the daemon emits one WRP EVENT approximately every 10 seconds

#### Scenario: Missing env var uses default
- **WHEN** `APPARMOR_SIM_INTERVAL_SEC` is not set
- **THEN** the daemon emits events every 30 seconds

---

### Requirement: On-demand emission via RBUS method
The apparmor-simulator SHALL register the RBUS method `Device.AppArmor.SimulateEvent()` via `rbus_regDataElements()` with a `methodHandler` callback. When invoked, the callback SHALL call `emit_apparmor_event()` immediately and return `RBUS_ERROR_SUCCESS`. If RBUS registration fails, the daemon SHALL log a WARNING and continue operating (timer-based emission still functions).

#### Scenario: RBUS method invoked
- **WHEN** a caller invokes `Device.AppArmor.SimulateEvent()` via rbuscli or rbus client
- **THEN** the daemon emits one WRP EVENT immediately
- **THEN** the method returns `RBUS_ERROR_SUCCESS`

#### Scenario: RBUS unavailable at startup
- **WHEN** `rbus_open()` fails
- **THEN** the daemon logs WARNING and continues without RBUS method registration
- **THEN** timer-based emission proceeds normally

---

### Requirement: WRP EVENT message format
Each emitted event SHALL be a `WRP_MSG_TYPE__EVENT` with:
- `source`: `mac:{device_mac_hex}/apparmor-simulator` where `device_mac_hex` is the MAC of `erouter0` with colons stripped
- `dest`: `event:apparmor/{severity}/mac:{device_mac_hex}` where `{severity}` is the lowercase AppArmor disposition (`denied`, `audit`, or `allowed`)
- `content_type`: `application/json`
- `payload`: JSON object conforming to the AppArmor event schema (see below)

#### Scenario: Event emitted with DENIED disposition
- **WHEN** the fixture at the current index has `apparmor: DENIED`
- **THEN** the WRP dest is `event:apparmor/denied/mac:{device_mac_hex}`
- **THEN** the JSON payload contains `"apparmor": "DENIED"` and `"simulated": true`

#### Scenario: MAC read from sysfs
- **WHEN** `/sys/class/net/erouter0/address` contains `aa:bb:cc:dd:ee:ff`
- **THEN** the WRP source is `mac:aabbccddeeff/apparmor-simulator`
- **THEN** the dest contains `mac:aabbccddeeff`

---

### Requirement: AppArmor event JSON schema
The event payload SHALL be a JSON object with the following fields:
- `timestamp` (string, RFC3339): emission time
- `device_id` (string): `mac:{device_mac_hex}`
- `apparmor` (string): `DENIED`, `AUDIT`, or `ALLOWED`
- `operation` (string): kernel operation (e.g., `open`, `exec`, `connect`)
- `profile` (string): AppArmor profile path (e.g., `/usr/sbin/dnsmasq`)
- `name` (string): resource name
- `pid` (number): simulated PID
- `comm` (string): process basename
- `requested_mask` (string): requested access mask
- `denied_mask` (string): denied access mask
- `fsuid` (number): filesystem UID
- `ouid` (number): object owner UID
- `simulated` (boolean): always `true`

#### Scenario: Valid JSON emitted
- **WHEN** an event is emitted
- **THEN** the payload is valid JSON containing all required fields
- **THEN** `simulated` is `true`

---

### Requirement: Logging via syslog
The apparmor-simulator SHALL log via `syslog` (opened with `openlog`, facility `LOG_DAEMON`), emitting `LOG_INFO` for lifecycle and emission events and `LOG_ERR` for failures. It SHALL NOT depend on cimplog for its own logging (cimplog remains only as a transitive link dependency of libwrp-c).

#### Scenario: Startup logged to syslog
- **WHEN** the daemon starts
- **THEN** it writes an `apparmor-simulator[pid]:` INFO line to the container syslog

---

### Requirement: Systemd service configuration
The `apparmor-simulator.service` unit SHALL declare `After=rbus.service parodus.service` and `Restart=always` with `RestartSec=15`.

#### Scenario: Service restarts on crash
- **WHEN** the apparmor-simulator process exits unexpectedly
- **THEN** systemd restarts it after 15 seconds

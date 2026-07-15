# event-sink

Generic XMiDT webhook consumer. Registers a configurable webhook with Caduceus
via Argus-backed registration and logs all matching WRP events as structured JSON.
Useful for testing any event type flowing through the XMiDT pipeline.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ARGUS_URL` | `http://webpa:6600` | Argus base URL (Argus runs inside the webpa container) |
| `ARGUS_BASIC_AUTH` | _(required)_ | Base64-encoded `user:pass` for Argus (`dXNlcjpwYXNz` = `user:pass` in dev) |
| `WEBHOOK_URL` | `http://event-sink:8080/webhook` | This service's webhook URL (must be reachable by Caduceus) |
| `WEBHOOK_EVENTS_REGEX` | `apparmor/.*` | Caduceus events filter regex — change to capture different event types |
| `WEBHOOK_DEVICE_MATCHER` | `.*` | Caduceus device_id filter regex |
| `WEBHOOK_SECRET` | _(required)_ | HMAC-SHA256 secret shared with Caduceus for `X-Webpa-Signature` validation |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |

## Starting the service

### Via vcpe (recommended)

Declare event-sink in your deployment manifest and bring it up with vcpe:

```bash
vcpe up --manifest manifests/example.yaml
```

The manifest's `event-sink` service entry handles env vars and network attachment
automatically. See `manifests/example.yaml` for a working example.

### Standalone (development / unit testing)

For isolated testing, the compose file requires `${IFACE_MGMT_NETWORK}` from the
webpa rendered `compose.env`:

```bash
podman compose \
  --env-file services/webpa/compose.env \
  -f services/event-sink/compose.yaml \
  up -d
```

## Capturing different event types

Change `WEBHOOK_EVENTS_REGEX` to match any Caduceus-routed event:

```bash
# Capture all events
WEBHOOK_EVENTS_REGEX=.* podman compose ...

# Capture only device-status events
WEBHOOK_EVENTS_REGEX=device-status.* podman compose ...

# Capture AppArmor denied events only
WEBHOOK_EVENTS_REGEX=apparmor/denied.* podman compose ...
```

## Example log output

```json
{"time":"2026-07-09T10:30:15Z","level":"INFO","msg":"webhook registered","events_regex":"apparmor/.*","device_matcher":".*","argus_item_id":"7e8c5f..."}
{"time":"2026-07-09T10:30:45Z","level":"INFO","msg":"event received","dest":"event:apparmor/denied/mac:aabbccddeeff","device_id":"mac:aabbccddeeff","content_type":"application/json","payload_size":312}
```

## Development

```bash
cd services/event-sink
go test ./...    # run unit tests
go build ./...   # verify compilation
```

## Architecture

- Webhook registration is Argus-backed (PUT to `http://webpa:6600/store/webhooks/{sha256(url)}`), durable across Caduceus restarts.
- TTL is 12 hours; a background goroutine refreshes every 6 hours.
- All incoming webhook POSTs are validated with `X-Webpa-Signature: sha256=<hmac>`.
- Received events are logged as structured JSON to stdout — no persistent storage.

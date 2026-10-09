# telemetry-journal

Standalone compose service for [sherloQ's telemetry journal](../../temp/sherloq/Frontend/telemetry_journal/)
— the Flask/DuckDB app that renders sherloq-agent telemetry (dashboards,
explorer). It is **not** a manifest-declared vcpe service type; it's a plain
compose project you bring up alongside a deployment that already has
event-sink running, so event-sink can relay matching webhook events into it.

See the parent change's design notes for why: `openspec/changes/sherloq-telemetry-journal-relay/design.md`.

## Starting the service

Bring up your vcpe deployment (with event-sink) first, so its `mgmt` network
already exists, then:

```bash
podman compose \
  --env-file services/telemetry-journal/compose.env \
  -f services/telemetry-journal/compose.yaml \
  up -d
```

`IFACE_MGMT_NETWORK` in `compose.env` must match the mgmt network name for
that deployment (see `services/event-sink/README.md` for how to find it).
The container joins that network with the alias `telemetry-journal`, so
event-sink (and anything else on the same network) can reach it at
`http://telemetry-journal:5007`. The UI itself is also published to the host
at `http://localhost:5007`.

## One-time setup: minting an ingest token

telemetry-journal's `/ingest` endpoint has no ingest-token environment
variable — tokens are minted at runtime and returned once, in plaintext:

```bash
curl -X POST http://localhost:5007/api/sources/tokens \
  -H 'Content-Type: application/json' \
  -d '{"label":"event-sink-relay"}'
```

Copy the `token` value from the response into event-sink's
`JOURNAL_INGEST_TOKEN` environment variable. Minting a token latches
telemetry-journal's remote-ingest enforcement on **permanently** — revoking
the token later does not reopen unauthenticated remote ingest; use
`POST /api/sources/tokens/enforcement` with `{"enforced": false}` if you need to
deliberately reopen it.

## Notes

- Built from telemetry_journal's existing Containerfile, unmodified.
- `SHERLOQ_ROLE=monolith` (set in `compose.env`) runs ingest and the explorer
  UI in one process — the right role for a single relay feeding a single
  journal instance.
- `/data` is a named volume (`telemetry-journal-data`), so ingested telemetry,
  dashboards, and minted ingest tokens survive container restarts.

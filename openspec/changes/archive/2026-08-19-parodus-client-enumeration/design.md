## Context

The CPE-to-WebPA diagnostic already queries a named Parodus client through a source-owned Scytale WRP Retrieve. The Gateway image now includes a patched Parodus route at `<device>/parodus/client-list`, which returns a bounded, lexicographically ordered list of receive-enabled client names. The control plane must preserve its no-container-runtime diagnostic contract and must not serialize credentials or unbounded raw WRP data.

## Goals / Non-Goals

**Goals:**
- Expose the registered client list through `vcpe diagnose --to parodus` for a selected Gateway replica.
- Keep all Scytale addressing, authentication, device identity derivation, WRP encoding, and MessagePack decoding inside the workload-local health daemon.
- Return structured JSON and deterministic human output with validated client names and truncation state.

**Non-Goals:**
- Enumerating send-only or disconnected libparodus clients that Parodus does not register.
- Adding a generic arbitrary-WRP diagnostic facility, container exec, or a dependency on a WebPA deployment for this source-local query.
- Changing the Parodus patch or using the client list as proof that an application is healthy beyond its receive-enabled registration.

## Decisions

### Use a dedicated source-local journey selected by `--to parodus`

The CLI maps `--to parodus` to a `parodus-clients` journey supported by Gateway. Its diagnostic graph has the selected Gateway as source and the semantic Parodus service as target, avoiding a misleading WebPA dependency. Existing `--to webpa`, `webhook`, and `callback` behavior remains unchanged.

An additional boolean list flag was rejected because `--to` already selects a mutually exclusive diagnostic journey and keeps cross-journey validation explicit.

### Reuse Scytale WRP transport and expose structured bounded fields

The health daemon sends a correlated Retrieve to `<device>/parodus/client-list`, validates the MessagePack WRP response, and decodes only `client-list` and `truncated`. It validates no more than 64 client-service identifiers, preserves the response ordering only after confirming it is lexicographically sorted, and copies the list into structured `parodusClients` and `parodusClientsTruncated` result fields.

Packing the list into diagnostic evidence was rejected because evidence is limited to eight short values and cannot safely represent the patched route's bounded 64-client response.

### Treat unavailable enumeration as inconclusive

Inactive Parodus, unavailable configuration, HTTP/auth errors, malformed MessagePack, correlation mismatch, invalid JSON, malformed names, unsorted entries, or an over-limit list produce an `unknown` observation. A valid response is `passed` whether or not it is empty; `truncated` reports that the list is incomplete rather than making the route fail.

## Risks / Trade-offs

- [The deployed Gateway lacks the patch] → Capability discovery still succeeds, but the journey reports bounded unknown evidence rather than exposing a raw response or failing unrelated diagnostics.
- [Registry semantics exclude send-only clients] → Documentation and output describe registered receive-enabled clients, not all processes that might use Parodus.
- [Result model adds journey-specific fields] → Fields are omitted for existing journeys and validated only for `parodus-clients`.
- [A recreated Gateway changes device identity] → The source derives the current identity from its existing health configuration at request time.

## Migration Plan

1. Build and deploy the health daemon with the new journey to Gateway images that contain the Parodus patch.
2. Recreate affected Gateway containers through the normal deployment flow.
3. Verify `vcpe diagnose --name <deployment> --from <gateway> --to parodus --json` returns the same bounded list as a direct source-owned WRP retrieval.
4. Roll back by redeploying the prior health-daemon image; the new CLI target is additive and does not alter existing diagnostic routes.
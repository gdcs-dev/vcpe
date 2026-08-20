## Context

The existing `cpe-webpa-callback` journey composes passive CPE, WebPA, subscriber-registration, routing, and receipt evidence around one explicitly authorized marked event. Gateway supplies the event through `/run/apparmor-simulator-diagnostic.sock`, a root-only source-local Unix socket that validates the selected client service, destination, local device identity, and correlation ID before constructing the WRP event.

XB10 installs and starts a packaged `apparmor-simulator` that exposes the same socket path and protocol. Its simulator, Parodus, and HermesFS services are active, and the socket is root-only. However, the XB10 health daemon does not declare `cpe-webpa-callback` or the socket environment variable, and the provider registry permits only Gateway as a callback source.

## Goals / Non-Goals

**Goals:**

- Support XB10 in the existing bounded `cpe-webpa-callback` journey with the same graph, validation, consent, correlation, and redaction behavior as Gateway.
- Enable XB10 only through the known, fixed root-only AppArmor simulator socket.
- Keep unsupported sources fail-closed and preserve Gateway behavior.
- Prove capability advertisement, provider selection, exact emitter protocol, and a consent-gated XB10 deployed path.

**Non-Goals:**

- Add a new journey, CLI flag, manifest field, or arbitrary source-event interface.
- Permit direct control-plane access to Parodus, Scytale, Talaria, WRP payloads, container exec, credentials, sockets chosen by an operator, or callback URLs.
- Change normal simulator traffic, webhook registration, Caduceus routing, subscriber behavior, or Parodus inventory semantics.
- Guarantee compatibility with an XB10 simulator package that no longer implements the validated fixed protocol.

## Decisions

### Use the fixed simulator socket for both sources

The callback probe will treat `/run/apparmor-simulator-diagnostic.sock` as the sole supported active-event interface when explicitly supplied by `VCPE_CPE_ACTIVE_EVENT_SOCKET`. The existing request validation, bounded JSON body, deadline, and exact `accepted` response check remain unchanged.

This reuses the verified XB10 package contract and retains source-local ownership of WRP construction. A generic environment-selected Unix socket or direct Parodus sender was rejected because either would let deployment configuration broaden the event-injection boundary.

### Parameterize the callback provider by source type

`NewCPEWebPACallbackProvider` will accept a supported source type and produce the same expected graph for Gateway and XB10. Built-in type registration will install providers for both types, keyed by journey/source/target as it is today.

Duplicating providers or graphs would create two independently evolving definitions of correlation and failure semantics. The provider remains limited to `webpa` targets and `event-sink` subscribers.

### Advertise XB10 support only through its health-daemon integration

The XB10 systemd health-daemon unit will set the fixed socket environment variable and declare `cpe-webpa-callback`. The daemon then exposes the capability through its existing `/diagnostics` discovery endpoint and invokes the existing callback probe through its persisted loopback endpoint.

The control plane will not infer capability from image type or inspect a container. A missing or failing socket continues to produce an inconclusive or failed active-event boundary from the source-local probe rather than a false delivery result.

### Retain explicit active-event smoke isolation

Unit and protocol tests cover provider selection and the socket request contract without emitting deployed traffic. One opt-in XB10 smoke uses the existing explicit active-event consent command, a registration-conformant representative destination and device identity, and asserts one correlated receipt.

This keeps routine verification passive while still validating the actual packaged simulator integration after an operator deliberately enables active traffic.

## Risks / Trade-offs

- [The external XB10 package changes its private socket protocol] -> Keep the fixed path and strict request/response validation; run the opt-in deployed smoke against the packaged image before release.
- [The simulator socket is unavailable during startup or after a restart] -> The health daemon reports a source-local active-event failure; it does not synthesize delivery success or retry unboundedly.
- [An active test emits traffic] -> Preserve required `--allow-active-event`, one-event-per-invocation behavior, bounded fields, authoritative registration matching, and documented operator consent.
- [XB10 identity is unavailable early in boot] -> Reuse the existing lazy `erouter0` device-identity refresh before CPE prerequisites and event validation.

## Migration Plan

1. Build the updated XB10 image with the health-daemon unit declaring the fixed callback capability.
2. Reconcile or recreate XB10 replicas so systemd loads the updated unit and health endpoint capabilities.
3. Verify passive capability discovery reports `cpe-webpa-callback` before running an explicit active-event diagnostic.
4. Roll back by deploying the previous XB10 image or removing the callback unit configuration; no persisted-state or manifest migration is required.

## Open Questions

None. The live XB10 package has already been verified to expose the fixed socket, correlation field, accepted response, and source-local WRP marker behavior required by this design.
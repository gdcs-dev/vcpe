## Context

The existing `webhook` diagnostic journey already runs a WebPA-local `WebhookProbe.Candidates` lookup against Argus's ownerless `webhooks` bucket. It uses those candidates only to match one event-sink subscriber's normalized callback identity, enforce freshness and conformance, and optionally send an active callback. The `parodus-clients` journey establishes the local pattern for a passive, source-owned diagnostic that returns a bounded structured list through the common result model and renderers.

This change reuses the WebPA-local Argus access path to inventory all registrations that Argus exposes, including registrations created by services or tools other than vCPE's event-sink. The architecture and user decisions are recorded in `architecture.md` and `decisions.md`.

## Goals / Non-Goals

**Goals:**
- Add a passive, WebPA-owned `webhooks` diagnostic journey that inventories the authoritative Argus `webhooks` bucket.
- Return a deterministic, strictly validated, non-secret registration list in both JSON and human output.
- Retain the existing diagnostic transport, persisted-endpoint resolution, central validation, redaction, and no-container-runtime contract.
- Keep the existing subscriber-focused `webhook` diagnostic behavior unchanged.

**Non-Goals:**
- Providing a generic Argus browser, arbitrary bucket query API, raw item download, pagination API, mutation, refresh, or deletion command.
- Proving a registration is fresh, conforms to a specific subscriber's intent, resolves, receives callbacks, or is currently selected by Caduceus.
- Discovering registrations unavailable from the selected WebPA instance's configured Argus endpoint.
- Returning secrets, credentials, authorization headers, HMAC values, raw stored items, query strings, URL userinfo, payloads, or container-runtime data.

## Decisions

### Use a WebPA-only passive journey

The CLI maps `--to webhooks` to a new `argus-webhooks` journey. It resolves one selected WebPA source and its persisted loopback endpoint but does not resolve a subscriber, CPE source, Parodus, or Caduceus participant. Capability discovery and invocation use the existing diagnostic HTTP protocol; the invocation carries no operator-controlled target URL, bucket, filter, or credentials.

This differs from `--to webhook`, which remains a multi-participant subscriber-registration and callback diagnostic. Sharing that journey would require a synthetic subscriber intent and would incorrectly imply per-subscriber conformance.

### Reuse compatible Argus decoding, with an inventory-specific bound

The WebPA probe continues to use the deployed ancla/chrysom client and `ancla.ItemToInternalWebhook`, not a hand-decoded Argus storage format. Add a distinct maximum of 64 inventory registrations. If the Argus bucket exceeds the bound, the probe reports an unknown/incomplete result and omits all registrations rather than returning a partial list that could be mistaken for the full inventory.

The existing eight-candidate limit remains for the focused `webhook` journey. Keeping a separate inventory limit prevents the broader read-only command from silently widening callback diagnosis behavior.

### Define an explicitly safe registration view

For every decoded item, construct a new serializable inventory entry rather than exposing `WebhookCandidate` directly. The view contains a stable fingerprint, normalized callback URL, lexicographically ordered event filters and device matchers, content type, `until`, optional TTL seconds, and `secretPresent`. Reject malformed callback identities, filter/matcher entries exceeding diagnostic text limits, invalid content types, invalid identifiers, or malformed timestamps before output.

The fingerprint is an opaque identifier, not a secret. `secretPresent` conveys configuration state only. Callback normalization removes userinfo, query, fragment, and default ports; no raw URL is rendered. Entries sort by fingerprint so output does not depend on Argus storage order.

### Extend the common result without overloading evidence

Add journey-specific structured fields such as `webhookRegistrations` to `Result` and `EndpointResponse`, similar to the Parodus client-list fields. A registration record is too rich and numerous for bounded evidence entries. Existing journeys omit these fields; validation accepts them only for `argus-webhooks` and validates every nested value.

The expected graph has WebPA as the source, Argus as the semantic target, and two passive stages: reachability and authenticated inventory retrieval. A valid empty bucket passes. Transport, authentication, decoding, and limit failures result in bounded `unknown` or `failed` observations according to existing diagnostic conventions, without raw server responses.

### Render structured output deterministically

JSON serializes the structured list and count through the common result renderer. Human output identifies the selected WebPA source and Argus target, then prints every registration in fingerprint order with safe field labels. It reports zero registrations explicitly. This follows the Parodus enumeration pattern while retaining webhook-specific field names.

## Risks / Trade-offs

- [Argus contains more than 64 entries] → Refuse the inventory as incomplete rather than make a partial list appear complete; report the bound in a safe observation.
- [A third-party item has malformed data] → Treat the response as an invalid inventory and do not serialize raw problematic fields.
- [Operators mistake presence for health] → Describe the command and output as an inventory; retain the existing `--to webhook` journey for conformance and callback delivery.
- [Argus ordering is nondeterministic] → Sort validated entries by opaque fingerprint before returning them.
- [WebPA has a different configured Argus view than another deployment] → Scope the result explicitly to the selected deployment and WebPA instance.

## Migration Plan

1. Build and deploy the updated `vcpe-healthd` with the `argus-webhooks` capability in WebPA images.
2. Recreate or update WebPA containers through the normal deployment path so their persisted loopback endpoints serve the new capability.
3. Verify JSON and human output against a bounded Argus bucket containing registrations from event-sink and an independent registrant.
4. Roll back by deploying the previous health daemon; the additive CLI target does not change existing journey behavior or persisted state.

## Open Questions

None. The selected behavior is an Argus-wide, bounded, safe inventory rather than a subscriber-only view.
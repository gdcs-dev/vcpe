package diagnostic

import (
	"context"
	"fmt"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

// Diagnose resolves one expected path, verifies source capability, collects
// active observations over loopback HTTP, and returns one validated safe graph.
func Diagnose(ctx context.Context, store *persist.Store, registry *Registry, client *Client, request ResolveRequest) (Result, error) {
	if request.Target == "callback" {
		if err := validateCallbackRequest(request); err != nil {
			return Result{}, err
		}
	}
	selection, err := Resolve(store, registry, request)
	if err != nil {
		return Result{}, err
	}
	expected, err := selection.Provider.Expected(ExpectedInput{Deployment: selection.Deployment, Source: selection.Source, Instance: selection.Instance, Target: selection.Target, Subscriber: selection.Subscriber})
	if err != nil {
		return Result{}, err
	}
	if expected.Journey == JourneyWebhook {
		invocation := Invocation{AllowActiveCallback: request.AllowActiveCallback, Event: request.Event, DeviceID: request.DeviceID}
		if err := invocation.ValidateFor(JourneyWebhook); err != nil {
			return Result{}, err
		}
		if invocation.AllowActiveCallback {
			return diagnoseWebhookActive(ctx, client, selection, expected, invocation)
		}
		return diagnoseWebhookPassive(ctx, client, selection, expected)
	}
	if expected.Journey == JourneyCPEWebPACallback {
		return diagnoseCPECallbackPrerequisites(ctx, client, selection, expected, request)
	}
	target := Target{Host: "127.0.0.1", Port: selection.Endpoint.HostPort}
	capabilities, err := client.Discover(ctx, target)
	if err != nil {
		return Result{}, err
	}
	if !supportsJourney(capabilities, expected.Journey) {
		return Result{}, fmt.Errorf("source service %q does not advertise diagnostic journey %q", selection.Source.Name, expected.Journey)
	}
	endpointResponse, err := client.Run(ctx, target, expected.Journey, Invocation{ClientService: request.ClientService})
	if err != nil {
		return Result{}, err
	}
	if endpointResponse.Journey != expected.Journey {
		return Result{}, fmt.Errorf("diagnostic endpoint returned journey %q: expected %q", endpointResponse.Journey, expected.Journey)
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, endpointResponse.Observations, endpointResponse.ObservedAt)
	if err != nil {
		return Result{}, fmt.Errorf("evaluate diagnostic response: %w", err)
	}
	observedAt := endpointResponse.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	result := Result{
		SchemaVersion:           SchemaVersion,
		Journey:                 expected.Journey,
		Source:                  expected.Source,
		Target:                  expected.Target,
		Metadata:                expected.Metadata,
		Nodes:                   expected.Nodes,
		Edges:                   expected.Edges,
		Observations:            observations,
		ParodusClients:          endpointResponse.ParodusClients,
		ParodusClientsTruncated: endpointResponse.ParodusClientsTruncated,
		FirstFailure:            firstFailure,
		ObservedAt:              observedAt,
	}
	return Sanitize(result)
}

func validateCallbackRequest(request ResolveRequest) error {
	if !request.AllowActiveEvent {
		return fmt.Errorf("active event consent is required")
	}
	if err := validateID("client service", request.ClientService); err != nil {
		return err
	}
	if err := validateID("subscriber", request.Subscriber); err != nil {
		return err
	}
	if err := validateInvocationText("event", request.Event, eventDestinationPattern); err != nil {
		return err
	}
	return validateInvocationText("device identity", request.DeviceID, deviceIdentityPattern)
}

// diagnoseCPECallbackPrerequisites collects all passive participant evidence
// before task 3.3 is allowed to create the single CPE-owned marked event.
func diagnoseCPECallbackPrerequisites(ctx context.Context, client *Client, selection Selection, expected ExpectedGraph, request ResolveRequest) (Result, error) {
	sourceTarget := Target{Host: "127.0.0.1", Port: selection.Endpoint.HostPort}
	sourceCapabilities, err := client.Discover(ctx, sourceTarget)
	if err != nil || !supportsJourney(sourceCapabilities, JourneyCPEWebPA) {
		return incompleteCPECallbackResult(expected, nil, nil, nil, "application-parodus", "CPE does not advertise passive CPE diagnostics")
	}
	cpe, err := client.Run(ctx, sourceTarget, JourneyCPEWebPA, Invocation{ClientService: request.ClientService})
	if err != nil || cpe.Journey != JourneyCPEWebPA {
		return incompleteCPECallbackResult(expected, nil, nil, nil, "application-parodus", "CPE diagnostic result was unavailable or invalid")
	}
	if !allPassed(cpe.Observations) {
		return callbackCPECallbackResult(expected, &cpe, nil, nil, Observation{})
	}

	subscriberTarget := Target{Host: "127.0.0.1", Port: selection.SubscriberEndpoint.HostPort}
	subscriberCapabilities, err := client.Discover(ctx, subscriberTarget)
	if err != nil || !supportsJourney(subscriberCapabilities, JourneyWebhookSubscriber) {
		return incompleteCPECallbackResult(expected, &cpe, nil, nil, "subscriber-intent", "subscriber does not advertise webhook diagnostics")
	}
	intent, err := client.SubscriberIntent(ctx, subscriberTarget)
	if err != nil {
		return incompleteCPECallbackResult(expected, &cpe, nil, nil, "subscriber-intent", "subscriber diagnostic result was unavailable or invalid")
	}

	webpaTarget := Target{Host: "127.0.0.1", Port: selection.TargetEndpoint.HostPort}
	webpaCapabilities, err := client.Discover(ctx, webpaTarget)
	if err != nil || !supportsJourney(webpaCapabilities, JourneyWebhook) {
		return incompleteCPECallbackResult(expected, &cpe, &intent, nil, "argus-reachability", "WebPA does not advertise webhook diagnostics")
	}
	webpa, err := client.RunWebhook(ctx, webpaTarget, intent, Invocation{})
	if err != nil || webpa.Journey != JourneyWebhook {
		return incompleteCPECallbackResult(expected, &cpe, &intent, nil, "argus-reachability", "WebPA diagnostic result was unavailable or invalid")
	}
	if !allPassed(webpa.Observations) {
		return callbackCPECallbackResult(expected, &cpe, &intent, &webpa, Observation{})
	}
	if !supportsJourney(sourceCapabilities, JourneyCPEWebPACallback) {
		return callbackCPECallbackResult(expected, &cpe, &intent, &webpa, callbackBoundary("active-event-acceptance", StateUnknown, ReasonActiveEventUnsupported, RemediationUseSupportedCPESource, "CPE does not advertise active callback diagnostics"))
	}
	if err := ValidateRepresentativeSelection(WebhookCandidate{EventFilters: []string{intent.EventFilter}, DeviceMatchers: []string{intent.DeviceMatcher}}, request.Event, request.DeviceID); err != nil {
		return callbackCPECallbackResult(expected, &cpe, &intent, &webpa, callbackBoundary("registration-conformant", StateFailed, ReasonRegistrationMismatch, RemediationAlignWebhookConfig, "selected event or device does not match the authoritative subscriber registration"))
	}
	correlationID, err := newDiagnosticCorrelationID()
	if err != nil {
		return incompleteCPECallbackResult(expected, &cpe, &intent, &webpa, "active-event-acceptance", "could not generate a diagnostic correlation identity")
	}
	active, err := client.RunCPECallback(ctx, sourceTarget, Invocation{ClientService: request.ClientService, Subscriber: request.Subscriber, AllowActiveEvent: true, Event: request.Event, DeviceID: request.DeviceID, CorrelationID: correlationID})
	if err != nil || active.Journey != JourneyCPEWebPACallback || !allPassed(active.Observations) {
		if active.Journey == JourneyCPEWebPACallback && len(active.Observations) > 0 {
			return callbackCPECallbackResult(expected, &active, &intent, &webpa, Observation{})
		}
		return callbackCPECallbackResult(expected, &cpe, &intent, &webpa, callbackBoundary("active-event-acceptance", StateUnknown, ReasonParticipantResultIncomplete, RemediationCheckParticipant, "CPE active event result was unavailable or invalid"))
	}
	routing, found, err := client.ObserveRouting(ctx, webpaTarget, correlationID)
	if err != nil || !found {
		return callbackCPECallbackResult(expected, &active, &intent, &webpa, callbackBoundary("routing-observation", StateUnknown, ReasonRoutingObservationUnavailable, RemediationCheckRoutingObservation, "Caduceus routing observation was unavailable"))
	}
	receipt, err := client.PollReceipt(ctx, subscriberTarget, correlationID)
	if err != nil || receipt.Source != "caduceus" {
		return callbackCPECallbackResult(expected, &active, &intent, &webpa, callbackBoundary("callback-receipt", StateUnknown, ReasonCaduceusReceiptMissing, RemediationCheckCaduceusDelivery, "subscriber callback receipt was unavailable or invalid"))
	}
	return completeCPECallbackResult(expected, active, intent, webpa, routing, receipt)
}

func completeCPECallbackResult(expected ExpectedGraph, cpe EndpointResponse, intent WebhookSubscriberIntent, webpa EndpointResponse, routing RoutingObservation, receipt Receipt) (Result, error) {
	observations := make([]Observation, len(expected.Edges))
	byEdge := make(map[string]Observation, len(cpe.Observations)+len(webpa.Observations))
	for _, observation := range cpe.Observations {
		byEdge[observation.EdgeID] = observation
	}
	for _, observation := range webpa.Observations {
		byEdge[observation.EdgeID] = observation
	}
	for index, edge := range expected.Edges {
		observation, ok := byEdge[edge.ID]
		if !ok {
			observation = Observation{EdgeID: edge.ID, State: StateUnknown, ReasonID: ReasonParticipantResultIncomplete, RemediationID: RemediationCheckParticipant, Message: "participant diagnostic result was unavailable or invalid", ObservedAt: cpe.ObservedAt}
		}
		if edge.ID == "subscriber-intent" {
			observation = passedWebhookObservation(edge.ID, intent.ObservedAt, nil)
		}
		if edge.ID == "routing-observation" {
			observation = Observation{EdgeID: edge.ID, State: StatePassed, ObservedAt: routing.ObservedAt}
		}
		if edge.ID == "callback-receipt" {
			observation = Observation{EdgeID: edge.ID, State: StatePassed, Evidence: []Evidence{{Key: "http-status", Value: fmt.Sprint(receipt.HTTPStatus)}}, ObservedAt: receipt.AcceptedAt}
		}
		observations[index] = observation
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, observations, latestObservationTime(observations))
	if err != nil {
		return Result{}, err
	}
	return Sanitize(Result{SchemaVersion: SchemaVersion, Journey: expected.Journey, Source: expected.Source, Target: expected.Target, Metadata: expected.Metadata, Nodes: expected.Nodes, Edges: expected.Edges, Observations: observations, FirstFailure: firstFailure, ObservedAt: latestObservationTime(observations)})
}

func incompleteCPECallbackResult(expected ExpectedGraph, cpe *EndpointResponse, intent *WebhookSubscriberIntent, webpa *EndpointResponse, boundary, message string) (Result, error) {
	return callbackCPECallbackResult(expected, cpe, intent, webpa, callbackBoundary(boundary, StateUnknown, ReasonParticipantResultIncomplete, RemediationCheckParticipant, message))
}

func callbackBoundary(edgeID string, state State, reason, remediation, message string) Observation {
	return Observation{EdgeID: edgeID, State: state, ReasonID: reason, RemediationID: remediation, Message: message}
}

func callbackCPECallbackResult(expected ExpectedGraph, cpe *EndpointResponse, intent *WebhookSubscriberIntent, webpa *EndpointResponse, boundary Observation) (Result, error) {
	now := time.Now().UTC()
	cpeObservations := map[string]Observation{}
	if cpe != nil {
		now = cpe.ObservedAt
		for _, observation := range cpe.Observations {
			switch observation.EdgeID {
			case "application-parodus", "talaria-dns", "talaria-transport", "talaria-authentication", "device-registration", "active-event-acceptance":
				cpeObservations[observation.EdgeID] = observation
			}
		}
	}
	webpaObservations := map[string]Observation{}
	if webpa != nil {
		now = webpa.ObservedAt
		for _, observation := range webpa.Observations {
			switch observation.EdgeID {
			case "argus-reachability", "argus-authentication", "registration-present", "registration-fresh", "registration-conformant":
				webpaObservations[observation.EdgeID] = observation
			}
		}
	}
	observations := make([]Observation, len(expected.Edges))
	for index, edge := range expected.Edges {
		observation := Observation{EdgeID: edge.ID, State: StateUnknown, ReasonID: ReasonParticipantResultIncomplete, RemediationID: RemediationCheckParticipant, Message: "participant diagnostic result was unavailable or invalid", ObservedAt: now}
		if candidate, ok := cpeObservations[edge.ID]; ok {
			observation = candidate
		}
		if edge.ID == "subscriber-intent" && intent != nil {
			observation = passedWebhookObservation(edge.ID, intent.ObservedAt, nil)
		}
		if candidate, ok := webpaObservations[edge.ID]; ok {
			observation = candidate
		}
		if edge.ID == "routing-observation" && boundary.EdgeID == "callback-receipt" {
			observation = Observation{EdgeID: edge.ID, State: StatePassed, ObservedAt: now}
		}
		if boundary.EdgeID == edge.ID {
			observation = boundary
			if observation.ObservedAt.IsZero() {
				observation.ObservedAt = now
			}
		}
		observations[index] = observation
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, observations, now)
	if err != nil {
		return Result{}, err
	}
	return Sanitize(Result{SchemaVersion: SchemaVersion, Journey: expected.Journey, Source: expected.Source, Target: expected.Target, Metadata: expected.Metadata, Nodes: expected.Nodes, Edges: expected.Edges, Observations: observations, FirstFailure: firstFailure, ObservedAt: latestObservationTime(observations)})
}

func diagnoseWebhookPassive(ctx context.Context, client *Client, selection Selection, expected ExpectedGraph) (Result, error) {
	subscriberTarget := Target{Host: "127.0.0.1", Port: selection.Endpoint.HostPort}
	subscriberCapabilities, err := client.Discover(ctx, subscriberTarget)
	if err != nil {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber diagnostic result was unavailable or invalid")
	}
	if !supportsJourney(subscriberCapabilities, "webhook-subscriber") {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber does not advertise webhook diagnostic intent")
	}
	intent, err := client.SubscriberIntent(ctx, subscriberTarget)
	if err != nil {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber diagnostic result was unavailable or invalid")
	}
	webpaTarget := Target{Host: "127.0.0.1", Port: selection.TargetEndpoint.HostPort}
	webpaCapabilities, err := client.Discover(ctx, webpaTarget)
	if err != nil {
		return incompleteWebhookResult(expected, &intent, nil, nil, "argus-reachability", "WebPA diagnostic result was unavailable or invalid")
	}
	if !supportsJourney(webpaCapabilities, JourneyWebhook) {
		return incompleteWebhookResult(expected, &intent, nil, nil, "argus-reachability", "WebPA does not advertise webhook diagnostics")
	}
	endpointResponse, err := client.RunWebhook(ctx, webpaTarget, intent, Invocation{})
	if err != nil {
		return incompleteWebhookResult(expected, &intent, &endpointResponse, nil, "argus-reachability", "WebPA diagnostic result was unavailable or invalid")
	}
	if endpointResponse.Journey != JourneyWebhook {
		return incompleteWebhookResult(expected, &intent, &endpointResponse, nil, "argus-reachability", "WebPA returned an invalid webhook diagnostic result")
	}
	observations, firstFailure, err := mergeWebhookPassiveObservations(expected.Edges, intent, endpointResponse)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		SchemaVersion: SchemaVersion,
		Journey:       expected.Journey,
		Source:        expected.Source,
		Target:        expected.Target,
		Metadata:      expected.Metadata,
		Nodes:         expected.Nodes,
		Edges:         expected.Edges,
		Observations:  observations,
		FirstFailure:  firstFailure,
		ObservedAt:    latestObservationTime(observations),
	}
	return Sanitize(result)
}

// diagnoseWebhookActive performs the two active WebPA operations serially.
// Each acknowledgement is followed by a subscriber receipt poll before the
// next operation can generate traffic.
func diagnoseWebhookActive(ctx context.Context, client *Client, selection Selection, expected ExpectedGraph, invocation Invocation) (Result, error) {
	subscriberTarget := Target{Host: "127.0.0.1", Port: selection.Endpoint.HostPort}
	subscriberCapabilities, err := client.Discover(ctx, subscriberTarget)
	if err != nil {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber diagnostic result was unavailable or invalid")
	}
	if !supportsJourney(subscriberCapabilities, "webhook-subscriber") {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber does not advertise webhook diagnostic intent")
	}
	intent, err := client.SubscriberIntent(ctx, subscriberTarget)
	if err != nil {
		return incompleteWebhookResult(expected, nil, nil, nil, "subscriber-intent", "subscriber diagnostic result was unavailable or invalid")
	}
	webpaTarget := Target{Host: "127.0.0.1", Port: selection.TargetEndpoint.HostPort}
	webpaCapabilities, err := client.Discover(ctx, webpaTarget)
	if err != nil {
		return incompleteWebhookResult(expected, &intent, nil, nil, "argus-reachability", "WebPA diagnostic result was unavailable or invalid")
	}
	if !supportsJourney(webpaCapabilities, JourneyWebhook) {
		return incompleteWebhookResult(expected, &intent, nil, nil, "argus-reachability", "WebPA does not advertise webhook diagnostics")
	}

	direct, err := client.RunWebhook(ctx, webpaTarget, intent, withWebhookPhase(invocation, WebhookActiveDirect))
	if err != nil {
		return incompleteWebhookResult(expected, &intent, &direct, nil, "callback-dns", "WebPA direct callback result was unavailable or invalid")
	}
	directReceipt, err := client.PollReceipt(ctx, subscriberTarget, direct.Active.CorrelationID)
	if err != nil {
		return incompleteWebhookResult(expected, &intent, &direct, []Observation{
			passedWebhookObservation("callback-dns", direct.ObservedAt, nil),
			passedWebhookObservation("callback-transport", direct.ObservedAt, nil),
		}, "callback-acceptance", "subscriber direct callback receipt was unavailable or invalid")
	}
	if directReceipt.Source != WebhookActiveDirect {
		return incompleteWebhookResult(expected, &intent, &direct, []Observation{
			passedWebhookObservation("callback-dns", direct.ObservedAt, nil),
			passedWebhookObservation("callback-transport", direct.ObservedAt, nil),
		}, "callback-acceptance", "subscriber returned an invalid direct callback receipt")
	}

	caduceus, err := client.RunWebhook(ctx, webpaTarget, intent, withWebhookPhase(invocation, WebhookActiveCaduceus))
	if err != nil {
		return incompleteWebhookResult(expected, &intent, &caduceus, []Observation{
			passedWebhookObservation("callback-dns", direct.ObservedAt, nil),
			passedWebhookObservation("callback-transport", direct.ObservedAt, nil),
			passedWebhookObservation("callback-acceptance", directReceipt.AcceptedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(direct.Active.HTTPStatus)}, {Key: "correlation-state", Value: "recorded"}}),
		}, "caduceus-ingestion", "WebPA Caduceus result was unavailable or invalid")
	}
	caduceusReceipt, err := client.PollReceipt(ctx, subscriberTarget, caduceus.Active.CorrelationID)
	if err != nil {
		return incompleteWebhookResult(expected, &intent, &direct, []Observation{
			passedWebhookObservation("callback-dns", direct.ObservedAt, nil),
			passedWebhookObservation("callback-transport", direct.ObservedAt, nil),
			passedWebhookObservation("callback-acceptance", directReceipt.AcceptedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(direct.Active.HTTPStatus)}, {Key: "correlation-state", Value: "recorded"}}),
			passedWebhookObservation("caduceus-ingestion", caduceus.ObservedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(caduceus.Active.HTTPStatus)}}),
		}, "caduceus-receipt", "subscriber Caduceus receipt was unavailable or invalid")
	}
	if caduceusReceipt.Source != WebhookActiveCaduceus {
		return incompleteWebhookResult(expected, &intent, &direct, []Observation{
			passedWebhookObservation("callback-dns", direct.ObservedAt, nil),
			passedWebhookObservation("callback-transport", direct.ObservedAt, nil),
			passedWebhookObservation("callback-acceptance", directReceipt.AcceptedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(direct.Active.HTTPStatus)}, {Key: "correlation-state", Value: "recorded"}}),
			passedWebhookObservation("caduceus-ingestion", caduceus.ObservedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(caduceus.Active.HTTPStatus)}}),
		}, "caduceus-receipt", "subscriber returned an invalid Caduceus receipt")
	}

	observations, _, err := mergeWebhookPassiveObservations(expected.Edges, intent, direct)
	if err != nil {
		return Result{}, err
	}
	for index := range observations {
		switch observations[index].EdgeID {
		case "callback-dns", "callback-transport":
			observations[index] = passedWebhookObservation(observations[index].EdgeID, direct.ObservedAt, nil)
		case "callback-acceptance":
			observations[index] = passedWebhookObservation(observations[index].EdgeID, directReceipt.AcceptedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(direct.Active.HTTPStatus)}, {Key: "correlation-state", Value: "recorded"}})
		case "caduceus-ingestion":
			observations[index] = passedWebhookObservation(observations[index].EdgeID, caduceus.ObservedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(caduceus.Active.HTTPStatus)}})
		case "caduceus-receipt":
			observations[index] = passedWebhookObservation(observations[index].EdgeID, caduceusReceipt.AcceptedAt, []Evidence{{Key: "http-status", Value: fmt.Sprint(caduceusReceipt.HTTPStatus)}, {Key: "correlation-state", Value: "recorded"}})
		}
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, observations, caduceusReceipt.AcceptedAt)
	if err != nil {
		return Result{}, err
	}
	return Sanitize(Result{
		SchemaVersion: SchemaVersion,
		Journey:       expected.Journey,
		Source:        expected.Source,
		Target:        expected.Target,
		Metadata:      expected.Metadata,
		Nodes:         expected.Nodes,
		Edges:         expected.Edges,
		Observations:  observations,
		FirstFailure:  firstFailure,
		ObservedAt:    caduceusReceipt.AcceptedAt,
	})
}

func withWebhookPhase(invocation Invocation, phase string) Invocation {
	invocation.ActivePhase = phase
	return invocation
}

func passedWebhookObservation(edgeID string, observedAt time.Time, evidence []Evidence) Observation {
	return Observation{EdgeID: edgeID, State: StatePassed, Evidence: evidence, ObservedAt: observedAt}
}

func incompleteWebhookResult(expected ExpectedGraph, intent *WebhookSubscriberIntent, response *EndpointResponse, passed []Observation, edgeID, message string) (Result, error) {
	now := time.Now().UTC()
	webpaObservations := make(map[string]Observation)
	if response != nil {
		for _, observation := range response.Observations {
			webpaObservations[observation.EdgeID] = observation
		}
	}
	webpaReportedIncomplete := false
	for _, observation := range webpaObservations {
		if observation.State != StatePassed {
			webpaReportedIncomplete = true
			break
		}
	}
	passedObservations := make(map[string]Observation, len(passed))
	for _, observation := range passed {
		passedObservations[observation.EdgeID] = observation
	}
	observations := make([]Observation, len(expected.Edges))
	for index, edge := range expected.Edges {
		observation := Observation{EdgeID: edge.ID, State: StateUnknown, ReasonID: ReasonParticipantResultIncomplete, RemediationID: RemediationCheckParticipant, Message: "participant diagnostic result was unavailable or invalid", ObservedAt: now}
		if edge.ID == "subscriber-intent" && intent != nil {
			observation = passedWebhookObservation(edge.ID, intent.ObservedAt, nil)
		}
		if candidate, ok := passedObservations[edge.ID]; ok {
			observation = candidate
		}
		candidate, hasWebPAObservation := webpaObservations[edge.ID]
		if hasWebPAObservation && (edge.ID != edgeID || webpaReportedIncomplete) {
			observation = candidate
		}
		if edge.ID == edgeID && !webpaReportedIncomplete {
			observation = Observation{EdgeID: edge.ID, State: StateUnknown, ReasonID: ReasonParticipantResultIncomplete, RemediationID: RemediationCheckParticipant, Message: message, ObservedAt: now}
		}
		observations[index] = observation
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, observations, now)
	if err != nil {
		return Result{}, err
	}
	return Sanitize(Result{SchemaVersion: SchemaVersion, Journey: expected.Journey, Source: expected.Source, Target: expected.Target, Metadata: expected.Metadata, Nodes: expected.Nodes, Edges: expected.Edges, Observations: observations, FirstFailure: firstFailure, ObservedAt: latestObservationTime(observations)})
}

func mergeWebhookPassiveObservations(edges []Edge, intent WebhookSubscriberIntent, response EndpointResponse) ([]Observation, string, error) {
	webpaObservations := make(map[string]Observation, len(response.Observations))
	for _, observation := range response.Observations {
		switch observation.EdgeID {
		case "argus-reachability", "argus-authentication", "registration-present", "registration-fresh", "registration-conformant":
			webpaObservations[observation.EdgeID] = observation
		default:
			return nil, "", fmt.Errorf("webpa returned unexpected passive webhook observation %q", observation.EdgeID)
		}
	}
	observations := make([]Observation, len(edges))
	for index, edge := range edges {
		observation := Observation{EdgeID: edge.ID, ObservedAt: response.ObservedAt}
		switch edge.ID {
		case "subscriber-intent":
			observation.State = StatePassed
			observation.ObservedAt = intent.ObservedAt
		case "callback-dns", "callback-transport", "callback-acceptance", "caduceus-ingestion", "caduceus-receipt":
			observation.State = StateUnknown
			observation.ReasonID = ReasonActiveCallbackNotRequested
			observation.RemediationID = RemediationAllowActiveCallback
			observation.Message = "active callback diagnosis was not requested"
		default:
			var ok bool
			observation, ok = webpaObservations[edge.ID]
			if !ok {
				observation = Observation{EdgeID: edge.ID, State: StateUnknown, ReasonID: "webpa-observation-missing", Message: "WebPA did not return an observation", ObservedAt: response.ObservedAt}
			}
		}
		observations[index] = observation
	}
	return ApplyCausality(edges, observations, response.ObservedAt)
}

func latestObservationTime(observations []Observation) time.Time {
	latest := time.Time{}
	for _, observation := range observations {
		if observation.ObservedAt.After(latest) {
			latest = observation.ObservedAt
		}
	}
	if latest.IsZero() {
		return time.Now().UTC()
	}
	return latest
}

func supportsJourney(capabilities Capabilities, journey string) bool {
	for _, candidate := range capabilities.Journeys {
		if candidate == journey {
			return true
		}
	}
	return false
}

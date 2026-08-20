package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const cpeDiagnosticSocket = "/run/apparmor-simulator-diagnostic.sock"

// CPEActiveEventRequest contains the validated, bounded fields a CPE-owned
// sender needs to create one reserved diagnostic event.
type CPEActiveEventRequest struct {
	ClientService string
	Event         string
	DeviceID      string
	CorrelationID string
}

// CPECallbackProbe combines existing passive CPE evidence with a source-owned
// emitter. The emitter is intentionally injected by the workload integration;
// the control plane never supplies an executable path, credentials, or WRP.
type CPECallbackProbe struct {
	CPE             CPEWebPAProbe
	EmitActiveEvent func(context.Context, CPEActiveEventRequest) error
}

// NewCPECallbackProbeFromEnvironment enables the sole supported source-owned
// sender only when a CPE workload supplies its fixed diagnostic socket. Other
// values remain unsupported rather than accepting an arbitrary socket path.
func NewCPECallbackProbeFromEnvironment(timeout time.Duration) CPECallbackProbe {
	probe := CPECallbackProbe{CPE: NewCPEWebPAProbeFromEnvironment(timeout)}
	if os.Getenv("VCPE_CPE_ACTIVE_EVENT_SOCKET") == cpeDiagnosticSocket {
		probe.EmitActiveEvent = newCPEUnixEmitter(cpeDiagnosticSocket)
	}
	return probe
}

func newCPEUnixEmitter(socketPath string) func(context.Context, CPEActiveEventRequest) error {
	return func(ctx context.Context, request CPEActiveEventRequest) error {
		if err := validateClientService(request.ClientService); err != nil {
			return err
		}
		if err := validateInvocationText("event", request.Event, eventDestinationPattern); err != nil {
			return err
		}
		if err := validateInvocationText("device identity", request.DeviceID, deviceIdentityPattern); err != nil {
			return err
		}
		if err := validateCorrelationID(request.CorrelationID); err != nil {
			return err
		}
		body, err := json.Marshal(struct {
			ClientService string `json:"clientService"`
			Event         string `json:"event"`
			DeviceID      string `json:"deviceId"`
			CorrelationID string `json:"correlationId"`
		}{request.ClientService, request.Event, request.DeviceID, request.CorrelationID})
		if err != nil {
			return err
		}
		connection, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		if err != nil {
			return fmt.Errorf("connect CPE diagnostic source: %w", err)
		}
		defer connection.Close()
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetDeadline(deadline)
		}
		if _, err := connection.Write(body); err != nil {
			return fmt.Errorf("write CPE diagnostic request: %w", err)
		}
		response, err := io.ReadAll(io.LimitReader(connection, 16))
		if err != nil {
			return fmt.Errorf("read CPE diagnostic response: %w", err)
		}
		if strings.TrimSpace(string(response)) != "accepted" {
			return fmt.Errorf("CPE diagnostic source rejected marked event")
		}
		return nil
	}
}

// RunWithInvocation collects prerequisites and emits exactly one marked event
// only when all of them pass.
func (probe CPECallbackProbe) RunWithInvocation(ctx context.Context, invocation Invocation) EndpointResponse {
	now := time.Now().UTC()
	if err := invocation.ValidateFor(JourneyCPEWebPACallback); err != nil {
		return EndpointResponse{
			SchemaVersion: SchemaVersion,
			Journey:       JourneyCPEWebPACallback,
			ObservedAt:    now,
			Observations: []Observation{{
				EdgeID:        "application-parodus",
				State:         StateUnknown,
				ReasonID:      ReasonActiveEventRejected,
				RemediationID: RemediationCheckActiveEvent,
				Message:       "active event invocation is invalid",
				ObservedAt:    now,
			}},
		}
	}

	response := probe.CPE.RunWithInvocation(ctx, Invocation{ClientService: invocation.ClientService})
	response.Journey = JourneyCPEWebPACallback
	if !allPassed(response.Observations) {
		response.Observations = append(response.Observations, Observation{EdgeID: "active-event-acceptance", State: StateSkipped, ReasonID: ReasonPrerequisiteFailed, Message: "active event was not generated because a prerequisite did not pass", ObservedAt: response.ObservedAt})
		return response
	}
	if probe.EmitActiveEvent == nil {
		response.Observations = append(response.Observations, Observation{EdgeID: "active-event-acceptance", State: StateUnknown, ReasonID: ReasonActiveEventUnsupported, RemediationID: RemediationUseSupportedCPESource, Message: "selected CPE does not expose a diagnostic event source", ObservedAt: response.ObservedAt})
		return response
	}
	request := CPEActiveEventRequest{ClientService: invocation.ClientService, Event: invocation.Event, DeviceID: invocation.DeviceID, CorrelationID: invocation.CorrelationID}
	if err := probe.EmitActiveEvent(ctx, request); err != nil {
		response.Observations = append(response.Observations, Observation{EdgeID: "active-event-acceptance", State: StateFailed, ReasonID: ReasonActiveEventRejected, RemediationID: RemediationCheckActiveEvent, Message: "CPE did not accept the marked diagnostic event", ObservedAt: response.ObservedAt})
		return response
	}
	response.Observations = append(response.Observations, Observation{EdgeID: "active-event-acceptance", State: StatePassed, Evidence: []Evidence{{Key: "correlation-state", Value: "accepted"}}, ObservedAt: response.ObservedAt})
	response.ActiveEvent = &CPEActiveEventResult{Accepted: true}
	return response
}

func allPassed(observations []Observation) bool {
	for _, observation := range observations {
		if observation.State != StatePassed {
			return false
		}
	}
	return true
}

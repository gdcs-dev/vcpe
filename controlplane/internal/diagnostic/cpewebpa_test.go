package diagnostic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
)

type stubConnection struct{ io.ReadWriteCloser }

func (stubConnection) LocalAddr() net.Addr              { return stubAddress("local") }
func (stubConnection) RemoteAddr() net.Addr             { return stubAddress("remote") }
func (stubConnection) SetDeadline(time.Time) error      { return nil }
func (stubConnection) SetReadDeadline(time.Time) error  { return nil }
func (stubConnection) SetWriteDeadline(time.Time) error { return nil }

type stubAddress string

func (address stubAddress) Network() string { return "tcp" }
func (address stubAddress) String() string  { return string(address) }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func baseCPEWebPAProbe(t *testing.T, status int, body string) CPEWebPAProbe {
	t.Helper()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	return CPEWebPAProbe{
		TalariaURL:    "http://talaria:6200/api/v2/devices",
		ScytaleURL:    "http://scytale:6300/api/v3/device",
		Username:      "user",
		Password:      "secret-password",
		DeviceID:      "mac:001122334455",
		ParodusClient: "apparmor-simulator",
		Now:           func() time.Time { return now },
		ServiceState:  func(context.Context) string { return "active" },
		LookupHost:    func(context.Context, string) ([]string, error) { return []string{"10.0.0.2"}, nil },
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return stubConnection{ReadWriteCloser: nopReadWriteCloser{}}, nil
		},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Hostname() == "scytale" {
				return scytaleResponse(t, request, "online"), nil
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})},
	}
}

type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read([]byte) (int, error)        { return 0, io.EOF }
func (nopReadWriteCloser) Write(value []byte) (int, error) { return len(value), nil }
func (nopReadWriteCloser) Close() error                    { return nil }

func TestCPEWebPAProbeHealthyPathIncludesOnlineApplication(t *testing.T) {
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`)
	response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "apparmor-simulator"})
	if err := response.Validate(); err != nil {
		t.Fatalf("response: %v", err)
	}
	want := []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed}
	for index, state := range want {
		if response.Observations[index].State != state {
			t.Errorf("state %d = %q, want %q", index, response.Observations[index].State, state)
		}
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), probe.Password) {
		t.Fatal("response exposed password")
	}
}

func TestCPECallbackProbeEmitsOneEventAfterPrerequisites(t *testing.T) {
	probe := CPECallbackProbe{CPE: baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`)}
	var requests []CPEActiveEventRequest
	probe.EmitActiveEvent = func(_ context.Context, request CPEActiveEventRequest) error {
		requests = append(requests, request)
		return nil
	}
	response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", MaxCorrelationIDLength)})
	if err := response.Validate(); err != nil {
		t.Fatalf("response: %v", err)
	}
	if len(requests) != 1 || requests[0].CorrelationID != strings.Repeat("a", MaxCorrelationIDLength) || response.ActiveEvent == nil || !response.ActiveEvent.Accepted {
		t.Fatalf("requests = %+v, active event = %+v", requests, response.ActiveEvent)
	}
}

func TestCPECallbackProbeSkipsEventWhenPrerequisiteFails(t *testing.T) {
	probe := CPECallbackProbe{CPE: baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)}
	emits := 0
	probe.EmitActiveEvent = func(context.Context, CPEActiveEventRequest) error { emits++; return nil }
	response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", MaxCorrelationIDLength)})
	last := response.Observations[len(response.Observations)-1]
	if emits != 0 || last.State != StateSkipped || last.ReasonID != ReasonPrerequisiteFailed {
		t.Fatalf("emits = %d, active observation = %+v", emits, last)
	}
}

func TestCPECallbackProbeUsesOnlyFixedSocketFromEnvironment(t *testing.T) {
	t.Setenv("VCPE_CPE_ACTIVE_EVENT_SOCKET", cpeDiagnosticSocket)
	if probe := NewCPECallbackProbeFromEnvironment(time.Second); probe.EmitActiveEvent == nil {
		t.Fatal("fixed CPE diagnostic socket did not enable active event emitter")
	}
	t.Setenv("VCPE_CPE_ACTIVE_EVENT_SOCKET", "/run/other-diagnostic.sock")
	if probe := NewCPECallbackProbeFromEnvironment(time.Second); probe.EmitActiveEvent != nil {
		t.Fatal("arbitrary CPE diagnostic socket enabled active event emitter")
	}
}

func TestCPEUnixEmitterSendsOnlyBoundedSelection(t *testing.T) {
	socket, err := os.CreateTemp("/tmp", "vcpe-cpe-*.sock")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := socket.Name()
	if err := socket.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(socketPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		var request CPEActiveEventRequest
		if err := json.NewDecoder(io.LimitReader(connection, MaxInvocationBodySize)).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.ClientService != "apparmor-simulator" || request.Event != "apparmor/diagnostic" || request.DeviceID != "mac:001122334455" || request.CorrelationID != strings.Repeat("a", MaxCorrelationIDLength) {
			t.Errorf("request = %+v", request)
		}
		_, _ = io.WriteString(connection, "accepted\n")
	}()
	emit := newCPEUnixEmitter(socketPath)
	if err := emit(context.Background(), CPEActiveEventRequest{ClientService: "apparmor-simulator", Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", MaxCorrelationIDLength)}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	<-done
}

func TestCPEWebPAProbeReportsOfflineApplication(t *testing.T) {
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`)
	probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() == "scytale" {
			return scytaleResponse(t, request, "offline"), nil
		}
		return jsonHTTPResponse(http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`), nil
	})}
	response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "apparmor-simulator"})
	observation := response.Observations[0]
	if observation.State != StateFailed || observation.ReasonID != "parodus-client-offline" {
		t.Fatalf("application observation = %+v", observation)
	}
	if response.Observations[1].State != StatePassed {
		t.Fatalf("downstream diagnosis did not continue: %+v", response.Observations)
	}
}

func TestCPEWebPAProbeInvocationOverridesDefaultClient(t *testing.T) {
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`)
	probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() != "scytale" {
			return jsonHTTPResponse(http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`), nil
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Fatal(err)
		}
		if message.Destination != "mac:001122334455/parodus/service-status/HermesFS" {
			t.Fatalf("destination = %q", message.Destination)
		}
		message.Source, message.Destination = message.Destination, message.Source
		message.Payload = []byte(`{"service-status":"online"}`)
		var encoded []byte
		if err := wrp.NewEncoderBytes(&encoded, wrp.Msgpack).Encode(message); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(encoded)), Header: http.Header{}}, nil
	})}
	response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "HermesFS"})
	if response.Observations[0].State != StatePassed || response.Observations[0].Evidence[0].Value != "HermesFS" {
		t.Fatalf("application observation = %+v", response.Observations[0])
	}
}

func TestCPEWebPAProbeListsParodusClients(t *testing.T) {
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
	probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return parodusClientListResponse(t, request, `{"client-list":["apparmor-simulator","config"],"truncated":false}`, false), nil
	})}
	response := probe.RunParodusClients(context.Background(), Invocation{})
	if err := response.Validate(); err != nil {
		t.Fatalf("response: %v", err)
	}
	if response.Observations[0].State != StatePassed || response.ParodusClients == nil || strings.Join(*response.ParodusClients, ",") != "apparmor-simulator,config" || response.ParodusClientsTruncated == nil || *response.ParodusClientsTruncated {
		t.Fatalf("response = %+v", response)
	}
}

func TestCPEWebPAProbeRefreshesMissingDeviceIDForParodusClients(t *testing.T) {
	t.Setenv("VCPE_HEALTH_SERIAL", "001122334455")
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
	probe.DeviceID = ""
	probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return parodusClientListResponse(t, request, `{"client-list":["apparmor-simulator"],"truncated":false}`, false), nil
	})}
	response := probe.RunParodusClients(context.Background(), Invocation{})
	if response.Observations[0].State != StatePassed || response.ParodusClients == nil || strings.Join(*response.ParodusClients, ",") != "apparmor-simulator" {
		t.Fatalf("response = %+v", response)
	}
}

func TestCPEWebPAProbeInventoriesTalariaDevices(t *testing.T) {
	var requests []*http.Request
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334456","pending":1,"statistics":{"bytesSent":2,"messagesSent":3,"bytesReceived":4,"messagesReceived":5,"duplications":6,"connectedAt":"2026-08-19T12:00:00Z","upTime":"1m2s"},"secret":"never-emit"},{"id":"mac:001122334455","pending":0,"statistics":{"bytesSent":7,"messagesSent":8,"bytesReceived":9,"messagesReceived":10,"duplications":11,"connectedAt":"2026-08-19T11:00:00Z","upTime":"2m3s"},"websocket":"never-emit"}]}`)
	probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request)
		return jsonHTTPResponse(http.StatusOK, `{"devices":[{"id":"mac:001122334456","pending":1,"statistics":{"bytesSent":2,"messagesSent":3,"bytesReceived":4,"messagesReceived":5,"duplications":6,"connectedAt":"2026-08-19T12:00:00Z","upTime":"1m2s"},"secret":"never-emit"},{"id":"mac:001122334455","pending":0,"statistics":{"bytesSent":7,"messagesSent":8,"bytesReceived":9,"messagesReceived":10,"duplications":11,"connectedAt":"2026-08-19T11:00:00Z","upTime":"2m3s"},"websocket":"never-emit"}]}`), nil
	})}
	response := probe.RunTalariaDevices(context.Background(), Invocation{})
	if err := response.Validate(); err != nil {
		t.Fatalf("response: %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet || requests[0].URL.Path != "/api/v2/devices" {
		t.Fatalf("requests = %+v", requests)
	}
	username, password, ok := requests[0].BasicAuth()
	if !ok || username != probe.Username || password != probe.Password {
		t.Fatalf("basic auth = %q, %q, %t", username, password, ok)
	}
	if response.TalariaDevices == nil || len(*response.TalariaDevices) != 2 || (*response.TalariaDevices)[0].ID != "mac:001122334455" || (*response.TalariaDevices)[0].MessagesReceived != 10 {
		t.Fatalf("devices = %+v", response.TalariaDevices)
	}
	encoded, err := json.Marshal(response)
	if err != nil || strings.Contains(string(encoded), "never-emit") || strings.Contains(string(encoded), probe.Password) {
		t.Fatalf("encoded response = %s, %v", encoded, err)
	}
}

func TestCPEWebPAProbeInventoriesEmptyTalariaDevices(t *testing.T) {
	probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
	response := probe.RunTalariaDevices(context.Background(), Invocation{})
	if err := response.Validate(); err != nil {
		t.Fatalf("response: %v", err)
	}
	if response.TalariaDevices == nil || len(*response.TalariaDevices) != 0 || response.Observations[1].State != StatePassed {
		t.Fatalf("response = %+v", response)
	}
}

func TestCPEWebPAProbeTalariaInventoryFailures(t *testing.T) {
	limitDevices := make([]TalariaDevice, MaxTalariaDevices+1)
	for index := range limitDevices {
		limitDevices[index] = validTalariaDevice(fmt.Sprintf("mac:%012x", index))
	}
	limitBody, err := json.Marshal(struct {
		Devices []TalariaDevice `json:"devices"`
	}{Devices: limitDevices})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		probe      func(*CPEWebPAProbe)
		wantState  State
		wantReason string
	}{
		{name: "authentication", probe: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusUnauthorized, `{}`) }, wantState: StateFailed, wantReason: "talaria-authentication-failed"},
		{name: "unexpected status", probe: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusServiceUnavailable, `{}`) }, wantState: StateUnknown, wantReason: "talaria-inventory-unavailable"},
		{name: "transport", probe: func(probe *CPEWebPAProbe) {
			probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
		}, wantState: StateFailed, wantReason: "talaria-request-failed"},
		{name: "malformed", probe: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusOK, `{`) }, wantState: StateUnknown, wantReason: "talaria-inventory-invalid"},
		{name: "invalid field", probe: func(probe *CPEWebPAProbe) {
			probe.HTTPClient = responseClient(http.StatusOK, `{"devices":[{"id":"bad"}]}`)
		}, wantState: StateUnknown, wantReason: "talaria-inventory-invalid"},
		{name: "null list", probe: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusOK, `{"devices":null}`) }, wantState: StateUnknown, wantReason: "talaria-inventory-invalid"},
		{name: "over limit", probe: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusOK, string(limitBody)) }, wantState: StateUnknown, wantReason: "talaria-inventory-limit"},
		{name: "oversized", probe: func(probe *CPEWebPAProbe) {
			probe.HTTPClient = responseClient(http.StatusOK, `{"devices":[],"padding":"`+strings.Repeat("x", MaxDiagnosticBodyBytes)+`"}`)
		}, wantState: StateUnknown, wantReason: "talaria-inventory-invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
			test.probe(&probe)
			response := probe.RunTalariaDevices(context.Background(), Invocation{})
			if err := response.Validate(); err != nil {
				t.Fatalf("response: %v", err)
			}
			index := 1
			if test.name == "transport" {
				index = 0
			}
			if response.TalariaDevices != nil || response.Observations[index].State != test.wantState || response.Observations[index].ReasonID != test.wantReason {
				t.Fatalf("response = %+v", response)
			}
		})
	}
}

func TestCPEWebPAProbeRejectsInvalidParodusClientList(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		mismatch bool
		want     string
	}{
		{name: "unsorted", payload: `{"client-list":["config","apparmor-simulator"],"truncated":false}`, want: "parodus-client-list-invalid"},
		{name: "missing truncation", payload: `{"client-list":[]}`, want: "parodus-client-list-invalid"},
		{name: "mismatched response", payload: `{"client-list":[],"truncated":false}`, mismatch: true, want: "scytale-response-mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
			probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return parodusClientListResponse(t, request, test.payload, test.mismatch), nil
			})}
			response := probe.RunParodusClients(context.Background(), Invocation{})
			if err := response.Validate(); err != nil {
				t.Fatalf("response: %v", err)
			}
			if response.Observations[0].State != StateUnknown || response.Observations[0].ReasonID != test.want || response.ParodusClients != nil || response.ParodusClientsTruncated != nil {
				t.Fatalf("response = %+v", response)
			}
		})
	}
}

func TestObserveParodusClientProtocolFailuresAreUnknown(t *testing.T) {
	tests := []struct {
		name      string
		transport roundTripFunc
		want      string
	}{
		{name: "authentication", transport: func(*http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusUnauthorized, `{}`), nil
		}, want: "scytale-authentication-failed"},
		{name: "transport", transport: func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}, want: "scytale-request-failed"},
		{name: "malformed", transport: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not-msgpack")), Header: http.Header{}}, nil
		}, want: "scytale-response-invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[]}`)
			probe.HTTPClient = &http.Client{Transport: test.transport}
			observation := probe.observeParodusClient(context.Background(), time.Now().UTC())
			if observation.State != StateUnknown || observation.ReasonID != test.want {
				t.Fatalf("observation = %+v", observation)
			}
		})
	}
}

func TestCPEWebPAProbeFailureStages(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*CPEWebPAProbe)
		failedAt  int
		wantState State
	}{
		{name: "dns", mutate: func(probe *CPEWebPAProbe) {
			probe.LookupHost = func(context.Context, string) ([]string, error) { return nil, errors.New("dns") }
		}, failedAt: 1, wantState: StateFailed},
		{name: "transport", mutate: func(probe *CPEWebPAProbe) {
			probe.DialContext = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connect") }
		}, failedAt: 2, wantState: StateFailed},
		{name: "authentication", mutate: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusUnauthorized, `{}`) }, failedAt: 3, wantState: StateFailed},
		{name: "malformed registry", mutate: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusOK, `{`) }, failedAt: 4, wantState: StateFailed},
		{name: "missing registration", mutate: func(probe *CPEWebPAProbe) { probe.HTTPClient = responseClient(http.StatusOK, `{"devices":[]}`) }, failedAt: 4, wantState: StateFailed},
		{name: "oversized registry", mutate: func(probe *CPEWebPAProbe) {
			probe.HTTPClient = responseClient(http.StatusOK, `{"devices":[],"padding":"`+strings.Repeat("x", MaxDiagnosticBodyBytes)+`"}`)
		}, failedAt: 4, wantState: StateFailed},
		{name: "HTTP timeout", mutate: func(probe *CPEWebPAProbe) {
			probe.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
		}, failedAt: 3, wantState: StateUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := baseCPEWebPAProbe(t, http.StatusOK, `{"devices":[{"id":"mac:001122334455"}]}`)
			test.mutate(&probe)
			response := probe.RunWithInvocation(context.Background(), Invocation{ClientService: "apparmor-simulator"})
			if response.Observations[test.failedAt].State != test.wantState {
				t.Fatalf("observation %d = %+v", test.failedAt, response.Observations[test.failedAt])
			}
			for index := test.failedAt + 1; index < len(response.Observations); index++ {
				if response.Observations[index].State != StateSkipped {
					t.Errorf("downstream state %d = %q, want skipped", index, response.Observations[index].State)
				}
			}
		})
	}
}

func TestCPEWebPAProbeUsesHealthConfigurationSources(t *testing.T) {
	t.Setenv("VCPE_TALARIA_DEVICES_URL", "http://custom-talaria:7200/api/v2/devices")
	t.Setenv("VCPE_TALARIA_BASIC_AUTH", "diagnostic-user:diagnostic-password")
	t.Setenv("VCPE_HEALTH_SERIAL", "001122334455")
	t.Setenv("VCPE_SCYTALE_URL", "http://custom-scytale:7300/api/v3/device")
	probe := NewCPEWebPAProbeFromEnvironment(time.Second)
	if probe.TalariaURL != "http://custom-talaria:7200/api/v2/devices" || probe.ScytaleURL != "http://custom-scytale:7300/api/v3/device" || probe.Username != "diagnostic-user" || probe.Password != "diagnostic-password" || probe.DeviceID != "mac:001122334455" || probe.ParodusClient != "" {
		t.Fatalf("probe configuration = %+v", probe)
	}
}

func responseClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
}

func scytaleResponse(t *testing.T, request *http.Request, status string) *http.Response {
	t.Helper()
	if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
		t.Fatalf("Scytale request = %s, content-type %q, accept %q", request.Method, request.Header.Get("Content-Type"), request.Header.Get("Accept"))
	}
	username, password, ok := request.BasicAuth()
	if !ok || username != "user" || password != "secret-password" {
		t.Fatalf("Scytale basic auth = %q/%q, %t", username, password, ok)
	}
	var requestMessage wrp.Message
	if err := json.NewDecoder(request.Body).Decode(&requestMessage); err != nil {
		t.Fatalf("decode Scytale request: %v", err)
	}
	if requestMessage.Type != wrp.RetrieveMessageType || requestMessage.Destination != "mac:001122334455/parodus/service-status/apparmor-simulator" {
		t.Fatalf("Scytale WRP request = %+v", requestMessage)
	}
	responseMessage := wrp.Message{
		Type:            wrp.RetrieveMessageType,
		Source:          requestMessage.Destination,
		Destination:     requestMessage.Source,
		TransactionUUID: requestMessage.TransactionUUID,
		Payload:         []byte(`{"service-status":"` + status + `"}`),
	}
	var encoded []byte
	if err := wrp.NewEncoderBytes(&encoded, wrp.Msgpack).Encode(responseMessage); err != nil {
		t.Fatalf("encode Scytale response: %v", err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(encoded)), ContentLength: int64(len(encoded)), Header: http.Header{"Content-Type": []string{wrpMsgpackContentType}}}
}

func parodusClientListResponse(t *testing.T, request *http.Request, payload string, mismatch bool) *http.Response {
	t.Helper()
	var requestMessage wrp.Message
	if err := json.NewDecoder(request.Body).Decode(&requestMessage); err != nil {
		t.Fatalf("decode Scytale request: %v", err)
	}
	if requestMessage.Type != wrp.RetrieveMessageType || requestMessage.Destination != "mac:001122334455/parodus/client-list" {
		t.Fatalf("Scytale WRP request = %+v", requestMessage)
	}
	transactionID := requestMessage.TransactionUUID
	if mismatch {
		transactionID = "mismatched"
	}
	responseMessage := wrp.Message{
		Type:            wrp.RetrieveMessageType,
		Source:          requestMessage.Destination,
		Destination:     requestMessage.Source,
		TransactionUUID: transactionID,
		Payload:         []byte(payload),
	}
	var encoded []byte
	if err := wrp.NewEncoderBytes(&encoded, wrp.Msgpack).Encode(responseMessage); err != nil {
		t.Fatalf("encode Scytale response: %v", err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(encoded)), ContentLength: int64(len(encoded)), Header: http.Header{"Content-Type": []string{wrpMsgpackContentType}}}
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

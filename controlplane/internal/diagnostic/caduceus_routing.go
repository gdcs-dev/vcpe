package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const caduceusRoutingPath = "/api/v4/diagnostics/correlation/"

// CaduceusRoutingProbe owns Caduceus credentials inside the WebPA workload.
// It exposes only one bounded correlation lookup to the diagnostic handler.
type CaduceusRoutingProbe struct {
	Endpoint   *url.URL
	Auth       string
	HTTPClient *http.Client
}

type caduceusRoutingRequest struct {
	CorrelationID string `json:"correlationId"`
}

type caduceusRoutingResponse struct {
	CorrelationID string    `json:"correlationId"`
	State         string    `json:"state"`
	ObservedAt    time.Time `json:"observedAt"`
}

// NewCaduceusRoutingProbeFromEnvironment uses the WebPA-local Caduceus
// endpoint and Basic credential. Neither is accepted from a caller or emitted.
func NewCaduceusRoutingProbeFromEnvironment(timeout time.Duration) (CaduceusRoutingProbe, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	endpoint, err := caduceusRoutingEndpoint(getenv("VCPE_CADUCEUS_URL", defaultCaduceusURL))
	if err != nil {
		return CaduceusRoutingProbe{}, err
	}
	auth := getenv("VCPE_CADUCEUS_BASIC_AUTH", defaultCaduceusAuth)
	if !strings.HasPrefix(auth, "Basic ") {
		auth = "Basic " + auth
	}
	return CaduceusRoutingProbe{
		Endpoint: endpoint,
		Auth:     auth,
		HTTPClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func caduceusRoutingEndpoint(rawURL string) (*url.URL, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("invalid local Caduceus diagnostic endpoint")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid local Caduceus diagnostic endpoint")
	}
	endpoint.Path = caduceusRoutingPath
	endpoint.RawPath = ""
	return endpoint, nil
}

// Handler returns a strict WebPA diagnostic subroute. It accepts only the
// opaque correlation ID and does not proxy arbitrary paths or methods.
func (probe CaduceusRoutingProbe) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if request.URL.Path != "/diagnostics/"+JourneyCPEWebPACallback+"/routing" {
			http.NotFound(writer, request)
			return
		}
		if request.ContentLength > MaxInvocationBodySize {
			http.Error(writer, "diagnostic request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		var input caduceusRoutingRequest
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, MaxInvocationBodySize))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(writer, "invalid diagnostic request", http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF || validateCorrelationID(input.CorrelationID) != nil {
			http.Error(writer, "invalid diagnostic request", http.StatusBadRequest)
			return
		}
		observation, found, err := probe.Lookup(request.Context(), input.CorrelationID)
		if err != nil {
			http.Error(writer, "Caduceus routing observation unavailable", http.StatusBadGateway)
			return
		}
		if !found {
			http.NotFound(writer, request)
			return
		}
		writeJSON(writer, observation)
	})
}

// Lookup queries Caduceus locally and translates its unversioned receipt into
// the versioned, strict diagnostic response shared with the control plane.
func (probe CaduceusRoutingProbe) Lookup(ctx context.Context, correlationID string) (RoutingObservation, bool, error) {
	if err := validateCorrelationID(correlationID); err != nil {
		return RoutingObservation{}, false, err
	}
	if probe.Endpoint == nil || probe.HTTPClient == nil || probe.Auth == "" {
		return RoutingObservation{}, false, fmt.Errorf("Caduceus routing probe is not configured")
	}
	endpoint := *probe.Endpoint
	endpoint.Path = caduceusRoutingPath + correlationID
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return RoutingObservation{}, false, err
	}
	request.Header.Set("Authorization", probe.Auth)
	response, err := probe.HTTPClient.Do(request)
	if err != nil {
		return RoutingObservation{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return RoutingObservation{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return RoutingObservation{}, false, fmt.Errorf("Caduceus routing endpoint returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > MaxInvocationBodySize {
		return RoutingObservation{}, false, fmt.Errorf("Caduceus routing response exceeds limit")
	}
	var raw caduceusRoutingResponse
	if err := decodeBounded(response.Body, MaxInvocationBodySize, &raw); err != nil {
		return RoutingObservation{}, false, fmt.Errorf("decode Caduceus routing response: %w", err)
	}
	observation := RoutingObservation{SchemaVersion: SchemaVersion, CorrelationID: raw.CorrelationID, State: raw.State, ObservedAt: raw.ObservedAt}
	if err := observation.Validate(correlationID); err != nil {
		return RoutingObservation{}, false, err
	}
	return observation, true, nil
}

package diagnostic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// JourneyHandler executes one trusted source-owned diagnostic journey.
type JourneyHandler func(context.Context, Invocation) EndpointResponse

// Server adds passive capability discovery and active journey routes to an
// existing per-instance HTTP handler.
type Server struct {
	Journeys map[string]JourneyHandler
	Timeout  time.Duration
}

// Handler returns a handler that preserves fallback routes such as /health.
func (server Server) Handler(fallback http.Handler) http.Handler {
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}
	timeout := server.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/diagnostics":
			if request.Method != http.MethodGet {
				writer.Header().Set("Allow", http.MethodGet)
				http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			journeys := make([]string, 0, len(server.Journeys))
			for journey := range server.Journeys {
				journeys = append(journeys, journey)
			}
			sort.Strings(journeys)
			writeJSON(writer, Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: journeys})
		case strings.HasPrefix(request.URL.Path, "/diagnostics/"):
			if request.Method != http.MethodPost {
				writer.Header().Set("Allow", http.MethodPost)
				http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if request.ContentLength > MaxInvocationBodySize {
				http.Error(writer, "diagnostic request body is too large", http.StatusRequestEntityTooLarge)
				return
			}
			journey := strings.TrimPrefix(request.URL.Path, "/diagnostics/")
			handler, ok := server.Journeys[journey]
			if !ok {
				http.NotFound(writer, request)
				return
			}
			ctx, cancel := context.WithTimeout(request.Context(), timeout)
			defer cancel()
			var invocation Invocation
			if request.ContentLength != 0 {
				limited := http.MaxBytesReader(writer, request.Body, MaxInvocationBodySize)
				decoder := json.NewDecoder(limited)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&invocation); err != nil {
					http.Error(writer, "invalid diagnostic request", http.StatusBadRequest)
					return
				}
				var extra any
				if err := decoder.Decode(&extra); err != io.EOF {
					http.Error(writer, "invalid diagnostic request", http.StatusBadRequest)
					return
				}
			}
			if err := invocation.Validate(); err != nil {
				http.Error(writer, "invalid diagnostic request", http.StatusBadRequest)
				return
			}
			response := handler(ctx, invocation)
			if err := response.Validate(); err != nil {
				http.Error(writer, "invalid diagnostic response", http.StatusInternalServerError)
				return
			}
			writeJSON(writer, response)
		default:
			fallback.ServeHTTP(writer, request)
		}
	})
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

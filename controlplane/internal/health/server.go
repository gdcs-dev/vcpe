package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Probe produces the current service-owned health response.
type Probe func(context.Context) Response

// Server exposes a Probe through the common GET /health protocol.
type Server struct {
	Probe Probe
}

// Handler returns an HTTP handler for the common health endpoint.
func (s Server) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/health" {
			http.NotFound(writer, request)
			return
		}
		response := Response{SchemaVersion: SchemaVersion, Status: StatusStarting, ObservedAt: time.Now().UTC()}
		if s.Probe != nil {
			response = s.Probe(request.Context())
			if response.SchemaVersion == "" {
				response.SchemaVersion = SchemaVersion
			}
			if response.ObservedAt.IsZero() {
				response.ObservedAt = time.Now().UTC()
			}
		}
		if response.Validate() != nil {
			response = Response{SchemaVersion: SchemaVersion, Status: StatusUnhealthy, ObservedAt: time.Now().UTC(), Checks: []Check{{Name: "health-probe", Status: StatusUnhealthy, Message: "invalid probe response"}}}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(response)
	})
}

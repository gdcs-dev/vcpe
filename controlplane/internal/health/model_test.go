package health

import (
	"strings"
	"testing"
	"time"
)

func validResponse() Response {
	return Response{
		SchemaVersion: SchemaVersion,
		Status:        StatusHealthy,
		ObservedAt:    time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC),
		Checks: []Check{{
			Name:   "service",
			Status: StatusHealthy,
		}},
	}
}

func TestResponseValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Response)
		wantErr string
	}{
		{name: "valid"},
		{
			name: "unsupported schema",
			mutate: func(response *Response) {
				response.SchemaVersion = "vcpe.dev/health/v2"
			},
			wantErr: "unsupported health schema version",
		},
		{
			name: "collector-only status",
			mutate: func(response *Response) {
				response.Status = "unknown"
			},
			wantErr: "invalid health status",
		},
		{
			name: "missing timestamp",
			mutate: func(response *Response) {
				response.ObservedAt = time.Time{}
			},
			wantErr: "health observation time is required",
		},
		{
			name: "duplicate check",
			mutate: func(response *Response) {
				response.Checks = append(response.Checks, response.Checks[0])
			},
			wantErr: "duplicate health check",
		},
		{
			name: "message too long",
			mutate: func(response *Response) {
				response.Checks[0].Message = strings.Repeat("x", MaxMessageLength+1)
			},
			wantErr: "message exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := validResponse()
			if test.mutate != nil {
				test.mutate(&response)
			}
			err := response.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestResponseValidateCheckLimit(t *testing.T) {
	response := validResponse()
	response.Checks = make([]Check, MaxChecks+1)
	for index := range response.Checks {
		response.Checks[index] = Check{Name: string(rune('a' + index)), Status: StatusHealthy}
	}
	if err := response.Validate(); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("Validate() error = %v, want maximum check error", err)
	}
}

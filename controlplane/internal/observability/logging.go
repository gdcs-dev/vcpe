package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Event struct {
	Timestamp   string `json:"ts"`
	Level       string `json:"level"`
	OperationID string `json:"operation_id,omitempty"`
	CustomerID  string `json:"customer_id,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Result      string `json:"result,omitempty"`
	Message     string `json:"message"`
}

func Log(event Event) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Level == "" {
		event.Level = "INFO"
	}

	b, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"level\":\"ERROR\",\"message\":\"failed to marshal log event\"}\n")
		return
	}
	fmt.Fprintln(os.Stderr, string(b))
}

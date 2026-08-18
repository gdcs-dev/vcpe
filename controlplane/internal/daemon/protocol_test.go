package daemon

import (
	"encoding/json"
	"testing"
)

func TestCommandRequestRoundTripsWebhookDiagnosticInputs(t *testing.T) {
	replica := 1
	want := CommandRequest{
		Command:             "diagnose",
		Name:                "edge",
		From:                "event-sink",
		To:                  "webhook",
		Replica:             &replica,
		AllowActiveCallback: true,
		Event:               "devices/diagnostic",
		DeviceID:            "mac:001122334455",
		OutputJSON:          true,
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got CommandRequest
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Command != want.Command || got.Name != want.Name || got.From != want.From || got.To != want.To || got.Replica == nil || *got.Replica != replica || !got.AllowActiveCallback || got.Event != want.Event || got.DeviceID != want.DeviceID || !got.OutputJSON {
		t.Fatalf("round-trip request = %+v", got)
	}
}

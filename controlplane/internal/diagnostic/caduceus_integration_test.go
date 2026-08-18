//go:build integration

package diagnostic

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
)

func TestCaduceusNotifyIntegration(t *testing.T) {
	endpoint := os.Getenv("VCPE_CADUCEUS_URL")
	username := os.Getenv("VCPE_CADUCEUS_USERNAME")
	password := os.Getenv("VCPE_CADUCEUS_PASSWORD")
	if endpoint == "" || username == "" || password == "" {
		t.Skip("set VCPE_CADUCEUS_URL, VCPE_CADUCEUS_USERNAME, and VCPE_CADUCEUS_PASSWORD to run")
	}

	message := wrp.Message{
		Type:            wrp.SimpleEventMessageType,
		Source:          "mac:001122334455/vcpe-diagnostic",
		Destination:     "event:apparmor/diagnostic/mac:001122334455",
		TransactionUUID: "9dfc7a20-53b1-4e93-a5df-98b6771d487a",
		ContentType:     "application/json",
		Payload:         []byte(`{"vcpe_diagnostic":"webhook-registration-callback-diagnostics","correlation_id":"9dfc7a20-53b1-4e93-a5df-98b6771d487a"}`),
	}
	var encoded bytes.Buffer
	if err := wrp.NewEncoder(&encoded, wrp.Msgpack).Encode(&message); err != nil {
		t.Fatalf("encode WRP message: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &encoded)
	if err != nil {
		t.Fatalf("create Caduceus request: %v", err)
	}
	request.Header.Set("Content-Type", wrpMsgpackContentType)
	request.SetBasicAuth(username, password)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send Caduceus request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("Caduceus status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
}

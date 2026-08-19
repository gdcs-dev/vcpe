package types_test

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

func TestRegisterAddsWebhookDiagnosticProvider(t *testing.T) {
	types.Register()
	if _, ok := diagnostic.DefaultRegistry().Lookup(diagnostic.JourneyWebhook, "event-sink", "webpa"); !ok {
		t.Fatal("webhook diagnostic provider is not registered")
	}
	if _, ok := diagnostic.DefaultRegistry().Lookup(diagnostic.JourneyParodusClients, "gateway", "parodus"); !ok {
		t.Fatal("Parodus client-list diagnostic provider is not registered")
	}
	if _, ok := diagnostic.DefaultRegistry().Lookup(diagnostic.JourneyArgusWebhooks, "webpa", "argus"); !ok {
		t.Fatal("Argus webhook inventory provider is not registered")
	}
}

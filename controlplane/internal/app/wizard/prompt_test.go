package wizard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

func TestPromptReturnsDefault(t *testing.T) {
	r := strings.NewReader("\n")
	var w bytes.Buffer
	got := Prompt(&w, r, "Name", "example")
	if got != "example" {
		t.Errorf("expected default %q, got %q", "example", got)
	}
}

func TestPromptReturnsInput(t *testing.T) {
	r := strings.NewReader("my-lab\n")
	var w bytes.Buffer
	got := Prompt(&w, r, "Name", "example")
	if got != "my-lab" {
		t.Errorf("expected %q, got %q", "my-lab", got)
	}
}

func TestPromptTrimsWhitespace(t *testing.T) {
	r := strings.NewReader("  value  \n")
	var w bytes.Buffer
	got := Prompt(&w, r, "Label", "default")
	if got != "value" {
		t.Errorf("expected trimmed %q, got %q", "value", got)
	}
}

func TestPromptBool(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true}, {"yes\n", true}, {"Y\n", true},
		{"n\n", false}, {"no\n", false}, {"\n", true}, // default true
	}
	for _, tc := range tests {
		r := strings.NewReader(tc.input)
		var w bytes.Buffer
		got := PromptBool(&w, r, "Enable?", true)
		if got != tc.want {
			t.Errorf("PromptBool(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestDefaultRepoUsesRegisteredDefaults(t *testing.T) {
	types.Register()
	want := map[string]string{
		"bng":               "ghcr.io/gdcs-dev/bng",
		"event-sink":        "ghcr.io/gdcs-dev/event-sink",
		"gateway":           "ghcr.io/gdcs-dev/gateway",
		"generic-container": "",
		"oktopus":           "",
		"webpa":             "ghcr.io/gdcs-dev/webpa",
		"xb10":              "ghcr.io/gdcs-dev/xb10",
	}

	registered := typeregistry.Registered()
	if len(registered) != len(want) {
		t.Fatalf("registered types = %q, want defaults for exactly %d built-ins", registered, len(want))
	}
	for _, serviceType := range registered {
		expected, ok := want[serviceType]
		if !ok {
			t.Errorf("registered type %q is missing from default-image expectations", serviceType)
			continue
		}
		t.Run(serviceType, func(t *testing.T) {
			if got := defaultRepo(serviceType); got != expected {
				t.Errorf("defaultRepo(%q) = %q, want %q", serviceType, got, expected)
			}
		})
	}
	if got := defaultRepo("unknown"); got != "" {
		t.Errorf("defaultRepo(unknown) = %q, want empty", got)
	}
}

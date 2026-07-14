package wizard

import (
	"bytes"
	"strings"
	"testing"
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

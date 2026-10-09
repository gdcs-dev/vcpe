package image

import "testing"

func TestServiceNameFromContext(t *testing.T) {
	if got := ServiceNameFromContext("services/bng"); got != "bng" {
		t.Fatalf("ServiceNameFromContext(services/bng) = %q, want bng", got)
	}
	if got := ServiceNameFromContext("services/bng/"); got != "bng" {
		t.Fatalf("ServiceNameFromContext(services/bng/) = %q, want bng", got)
	}
	if got := ServiceNameFromContext("."); got == "bng" {
		t.Fatalf("ServiceNameFromContext(.) unexpectedly matched bng")
	}
}

func TestRuntimeInitServicesExcludeRemovedType(t *testing.T) {
	if runtimeInitServices["routerd"] {
		t.Fatal("removed routerd service remains in the image-staging list")
	}
	for _, service := range []string{"bng", "event-sink", "gateway", "oktopus", "webconfig", "webpa", "xb10", "client"} {
		if !runtimeInitServices[service] {
			t.Errorf("missing runtime-init staging for %q", service)
		}
	}
}

func TestParsePlatform(t *testing.T) {
	goos, goarch, err := ParsePlatform("linux/arm64")
	if err != nil {
		t.Fatalf("ParsePlatform: %v", err)
	}
	if goos != "linux" || goarch != "arm64" {
		t.Fatalf("ParsePlatform(linux/arm64) = (%q, %q), want (linux, arm64)", goos, goarch)
	}

	if _, _, err := ParsePlatform("linux"); err == nil {
		t.Fatalf("expected error for platform missing arch")
	}
	if _, _, err := ParsePlatform(""); err == nil {
		t.Fatalf("expected error for empty platform")
	}
}

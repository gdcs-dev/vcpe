package imageref

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name  string
		image manifest.Image
		want  string
	}{
		{name: "empty repository", image: manifest.Image{}, want: ""},
		{name: "whitespace repository", image: manifest.Image{Repository: " \t ", Tag: "v2"}, want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example.test/workload"}, want: "example.test/workload:latest"},
		{name: "whitespace tag", image: manifest.Image{Repository: "example.test/workload", Tag: " \t "}, want: "example.test/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example.test/workload", Tag: "v2"}, want: "example.test/workload:v2"},
		{name: "preserves text", image: manifest.Image{Repository: " example.test/workload ", Tag: " v2 "}, want: " example.test/workload : v2 "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Format(test.image); got != test.want {
				t.Fatalf("Format(%#v) = %q, want %q", test.image, got, test.want)
			}
		})
	}
}

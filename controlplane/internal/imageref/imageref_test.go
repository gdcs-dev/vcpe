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
		{name: "empty repository", want: ""},
		{name: "whitespace repository", image: manifest.Image{Repository: "  "}, want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example.test/workload"}, want: "example.test/workload:latest"},
		{name: "whitespace tag", image: manifest.Image{Repository: "example.test/workload", Tag: "  "}, want: "example.test/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example.test/workload", Tag: "v2"}, want: "example.test/workload:v2"},
		{name: "preserves nonempty input", image: manifest.Image{Repository: " example.test/workload ", Tag: " v2 "}, want: " example.test/workload : v2 "},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Format(testCase.image); got != testCase.want {
				t.Errorf("Format(%+v) = %q, want %q", testCase.image, got, testCase.want)
			}
		})
	}
}

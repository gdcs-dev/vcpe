package ipam

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func TestPrimaryCIDR(t *testing.T) {
	tests := []struct {
		name    string
		network manifest.Network
		want    string
	}{
		{
			name: "IPv4 preferred",
			network: manifest.Network{
				IPv4: &manifest.AddressFamily{CIDR: "10.0.0.0/24"},
				IPv6: &manifest.AddressFamily{CIDR: "2001:db8::/64"},
			},
			want: "10.0.0.0/24",
		},
		{
			name:    "IPv6 fallback",
			network: manifest.Network{IPv6: &manifest.AddressFamily{CIDR: "2001:db8::/64"}},
			want:    "2001:db8::/64",
		},
		{
			name:    "empty families",
			network: manifest.Network{IPv4: &manifest.AddressFamily{}, IPv6: &manifest.AddressFamily{}},
			want:    "",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := PrimaryCIDR(testCase.network); got != testCase.want {
				t.Errorf("PrimaryCIDR(%+v) = %q, want %q", testCase.network, got, testCase.want)
			}
		})
	}
}

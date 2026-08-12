package ipam_test

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/ipam"
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
			name: "IPv6 fallback",
			network: manifest.Network{
				IPv4: &manifest.AddressFamily{},
				IPv6: &manifest.AddressFamily{CIDR: "2001:db8::/64"},
			},
			want: "2001:db8::/64",
		},
		{name: "no CIDR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ipam.PrimaryCIDR(test.network); got != test.want {
				t.Errorf("PrimaryCIDR() = %q, want %q", got, test.want)
			}
		})
	}
}

package rfmedium

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

type baselineAddress struct {
	deployment string
	mac        string
}

func BuildBaseline(deployments []plan.Deployment) (string, error) {
	addresses := []baselineAddress{}
	seenDeployments := map[string]bool{}
	seenMACs := map[string]string{}
	for _, deployment := range deployments {
		if deployment.Name == "" || seenDeployments[deployment.Name] {
			return "", fmt.Errorf("RF baseline has an empty or duplicate deployment %q", deployment.Name)
		}
		seenDeployments[deployment.Name] = true
		add := func(mac string) error {
			parsed, err := net.ParseMAC(mac)
			if err != nil || len(parsed) != 6 || parsed[0]&3 != 2 {
				return fmt.Errorf("RF baseline deployment %q has invalid managed MAC %q", deployment.Name, mac)
			}
			canonical := parsed.String()
			if owner := seenMACs[canonical]; owner != "" {
				return fmt.Errorf("RF baseline MAC %s is shared by %q and %q", canonical, owner, deployment.Name)
			}
			seenMACs[canonical] = deployment.Name
			addresses = append(addresses, baselineAddress{deployment: deployment.Name, mac: canonical})
			return nil
		}
		for _, service := range deployment.Services {
			for _, instance := range service.Instances {
				for _, radio := range instance.Radios {
					if err := add(radio.MAC); err != nil {
						return "", err
					}
					for _, vap := range radio.VAPs {
						if vap.Slot == 0 && vap.MAC == radio.MAC {
							continue
						}
						if err := add(vap.MAC); err != nil {
							return "", err
						}
					}
				}
			}
		}
	}
	if len(addresses) < 2 {
		return "", fmt.Errorf("RF baseline requires at least two managed addresses")
	}
	sort.Slice(addresses, func(left, right int) bool {
		return addresses[left].mac < addresses[right].mac
	})
	ids := make([]string, 0, len(addresses))
	links := make([]string, 0, len(addresses)*(len(addresses)-1)/2)
	for index, address := range addresses {
		ids = append(ids, fmt.Sprintf("%q", address.mac))
		for other := index + 1; other < len(addresses); other++ {
			snr := -100
			if address.deployment == addresses[other].deployment {
				snr = 35
			}
			links = append(links, fmt.Sprintf("(%d,%d,%d)", index, other, snr))
		}
	}
	return fmt.Sprintf("ifaces: { ids = [ %s ]; links = (%s); };\n", strings.Join(ids, ", "), strings.Join(links, ",")), nil
}

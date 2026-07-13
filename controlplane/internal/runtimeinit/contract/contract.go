package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// SupportedVersion is the runtime-init startup-contract schema version.
const SupportedVersion = "vcpe.dev/v1"

type Document struct {
	Version    string             `json:"version"`
	Service    string             `json:"service"`
	Deployment string             `json:"deployment"`
	Operation  OperationContext   `json:"operation"`
	Interfaces []InterfaceBinding `json:"interfaces"`
	Runtime    RuntimeContext     `json:"runtime"`
}

type OperationContext struct {
	ID string `json:"id,omitempty"`
}

type InterfaceBinding struct {
	Role     string `json:"role"`
	Name     string `json:"name,omitempty"`
	MAC      string `json:"mac,omitempty"`
	IPv4     string `json:"ipv4,omitempty"`
	IPv6     string `json:"ipv6,omitempty"`
	Gateway4 string `json:"gateway4,omitempty"`
	Gateway6 string `json:"gateway6,omitempty"`
}

type RuntimeContext struct {
	ConfigPath string `json:"configPath,omitempty"`
}

func Validate(doc Document) error {
	if strings.TrimSpace(doc.Version) == "" {
		return fmt.Errorf("startup contract version is required")
	}
	if doc.Version != SupportedVersion {
		return fmt.Errorf("unsupported startup contract version %q", doc.Version)
	}
	if strings.TrimSpace(doc.Service) == "" {
		return fmt.Errorf("startup contract service is required")
	}
	if strings.TrimSpace(doc.Deployment) == "" {
		return fmt.Errorf("startup contract deployment is required")
	}
	if len(doc.Interfaces) == 0 {
		return fmt.Errorf("startup contract interfaces are required")
	}
	for _, binding := range doc.Interfaces {
		if strings.TrimSpace(binding.Role) == "" {
			return fmt.Errorf("startup contract interface role is required")
		}
		if strings.TrimSpace(binding.Name) == "" {
			return fmt.Errorf("startup contract interface name is required")
		}
		if strings.TrimSpace(binding.MAC) == "" {
			return fmt.Errorf("startup contract interface mac is required")
		}
	}
	return nil
}

func Load(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read startup contract: %w", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("parse startup contract: %w", err)
	}
	if err := Validate(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// BuildForDeployment derives a per-service startup contract from the resolved
// plan. Interface identities come straight from the planner's resolved
// instances, so the runtime-init contract is guaranteed to agree with the
// control plane. Services with no instances (scaled to zero) are skipped.
func BuildForDeployment(opID string, dep plan.Deployment) map[string]Document {
	contracts := map[string]Document{}
	for _, svc := range dep.Services {
		if len(svc.Instances) == 0 {
			continue
		}
		inst := svc.Instances[0]
		interfaces := make([]InterfaceBinding, 0, len(inst.Interfaces))
		for _, iface := range inst.Interfaces {
			interfaces = append(interfaces, InterfaceBinding{
				Role:     iface.Role,
				Name:     iface.Device,
				MAC:      iface.MAC,
				IPv4:     iface.IPv4,
				IPv6:     iface.IPv6,
				Gateway4: iface.Gateway4,
				Gateway6: iface.Gateway6,
			})
		}
		contracts[svc.Name] = Document{
			Version:    SupportedVersion,
			Service:    svc.Name,
			Deployment: dep.Name,
			Operation:  OperationContext{ID: opID},
			Interfaces: interfaces,
			Runtime:    RuntimeContext{ConfigPath: fmt.Sprintf("runtime/%s", svc.Name)},
		}
	}
	return contracts
}

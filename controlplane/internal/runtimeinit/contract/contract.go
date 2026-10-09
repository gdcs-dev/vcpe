package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
)

// SupportedVersion is the runtime-init startup-contract schema version.
const SupportedVersion = "vcpe.dev/v1"

type Document struct {
	Version    string             `json:"version"`
	Service    string             `json:"service"`
	Deployment string             `json:"deployment"`
	Operation  OperationContext   `json:"operation"`
	Interfaces []InterfaceBinding `json:"interfaces"`
	Radios     []RadioBinding     `json:"radios,omitempty"`
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

type RadioBinding struct {
	Name                string                  `json:"name"`
	Device              string                  `json:"device"`
	MAC                 string                  `json:"mac"`
	Mode                string                  `json:"mode"`
	Medium              string                  `json:"medium,omitempty"`
	APBSSID             string                  `json:"apBssid,omitempty"`
	Candidates          []plan.RoamingCandidate `json:"candidates,omitempty"`
	MeshID              string                  `json:"meshId,omitempty"`
	MeshSAEFile         string                  `json:"meshSaeFile,omitempty"`
	Band                string                  `json:"band,omitempty"`
	WidthMHz            int                     `json:"widthMHz,omitempty"`
	VAPs                []VAPBinding            `json:"vaps,omitempty"`
	Network             string                  `json:"network"`
	SSID                string                  `json:"ssid"`
	Channel             int                     `json:"channel"`
	Security            string                  `json:"security,omitempty"`
	PassphraseSecretRef string                  `json:"passphraseSecretRef,omitempty"`
	PassphraseFile      string                  `json:"passphraseFile,omitempty"`
	Bridge              string                  `json:"bridge,omitempty"`
	Addressing          string                  `json:"addressing,omitempty"`
	DefaultRoute        bool                    `json:"defaultRoute,omitempty"`
}

type VAPBinding struct {
	Slot           int    `json:"slot"`
	Device         string `json:"device"`
	BSSID          string `json:"bssid"`
	Network        string `json:"network"`
	SSID           string `json:"ssid"`
	Security       string `json:"security"`
	Bridge         string `json:"bridge"`
	PassphraseFile string `json:"passphraseFile,omitempty"`
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
	if len(doc.Interfaces) == 0 && len(doc.Radios) == 0 {
		return fmt.Errorf("startup contract requires at least one interface or radio")
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
	radioNames := make(map[string]struct{}, len(doc.Radios))
	radioDevices := make(map[string]struct{}, len(doc.Radios))
	for _, binding := range doc.Radios {
		if strings.TrimSpace(binding.Name) == "" {
			return fmt.Errorf("startup contract radio name is required")
		}
		if _, exists := radioNames[binding.Name]; exists {
			return fmt.Errorf("startup contract radio name %q is duplicated", binding.Name)
		}
		radioNames[binding.Name] = struct{}{}
		if strings.TrimSpace(binding.Device) == "" {
			return fmt.Errorf("startup contract radio %q device is required", binding.Name)
		}
		if _, exists := radioDevices[binding.Device]; exists {
			return fmt.Errorf("startup contract radio device %q is duplicated", binding.Device)
		}
		radioDevices[binding.Device] = struct{}{}
		if binding.Medium != "" {
			if err := validateMediaRadioBinding(binding); err != nil {
				return err
			}
			continue
		}
		if len(binding.Candidates) > 0 {
			if binding.Mode != manifest.RadioModeStation || binding.Network == "" || binding.SSID == "" || binding.Addressing != "dhcp" || binding.Bridge != "" || binding.APBSSID != "" || len(binding.Candidates) < 2 || len(binding.VAPs) != 0 {
				return fmt.Errorf("startup contract roaming radio %q requires multiple eligible APs, a profile, and DHCP", binding.Name)
			}
			if binding.Security != manifest.WirelessOpen && (binding.Security != manifest.WirelessWPA2Personal && binding.Security != manifest.WirelessWPA3Personal || binding.PassphraseFile == "") {
				return fmt.Errorf("startup contract roaming radio %q has invalid security or credential file", binding.Name)
			}
			for _, candidate := range binding.Candidates {
				if candidate.BSSID == "" || candidate.Medium == "" || candidate.Band == "" || candidate.Channel < 1 {
					return fmt.Errorf("startup contract roaming radio %q has an incomplete candidate", binding.Name)
				}
			}
			continue
		}
		if strings.TrimSpace(binding.MAC) == "" || strings.TrimSpace(binding.Network) == "" || strings.TrimSpace(binding.SSID) == "" {
			return fmt.Errorf("startup contract radio %q mac, network, and ssid are required", binding.Name)
		}
		if binding.Channel < 1 || binding.Channel > 14 {
			return fmt.Errorf("startup contract radio %q channel must be between 1 and 14", binding.Name)
		}
		switch binding.Mode {
		case "ap":
			if strings.TrimSpace(binding.Bridge) == "" || binding.Addressing != "" || binding.DefaultRoute {
				return fmt.Errorf("startup contract AP radio %q requires a bridge and cannot declare addressing or a default route", binding.Name)
			}
		case "station":
			if binding.Bridge != "" || binding.Addressing != "dhcp" {
				return fmt.Errorf("startup contract station radio %q requires DHCP addressing and cannot declare a bridge", binding.Name)
			}
		default:
			return fmt.Errorf("startup contract radio %q mode must be ap or station", binding.Name)
		}
	}
	return nil
}

func validateMediaRadioBinding(binding RadioBinding) error {
	if binding.MAC == "" || binding.Band == "" || binding.Channel < 1 || binding.WidthMHz != 20 {
		return fmt.Errorf("startup contract radio %q requires MAC, band, channel, and 20 MHz width", binding.Name)
	}
	switch binding.Band {
	case "2.4ghz":
		if binding.Channel > 14 {
			return fmt.Errorf("startup contract radio %q has invalid 2.4 GHz channel %d", binding.Name, binding.Channel)
		}
	case "5ghz":
		if binding.Channel != 36 && binding.Channel != 40 && binding.Channel != 44 && binding.Channel != 48 &&
			binding.Channel != 149 && binding.Channel != 153 && binding.Channel != 157 && binding.Channel != 161 && binding.Channel != 165 {
			return fmt.Errorf("startup contract radio %q has invalid 5 GHz channel %d", binding.Name, binding.Channel)
		}
	case "6ghz":
		if binding.Channel < 5 || binding.Channel > 229 || (binding.Channel-5)%16 != 0 {
			return fmt.Errorf("startup contract radio %q has invalid 6 GHz PSC channel %d", binding.Name, binding.Channel)
		}
	default:
		return fmt.Errorf("startup contract radio %q has unknown band %q", binding.Name, binding.Band)
	}
	if binding.Mode == manifest.RadioModeMesh {
		if binding.MeshID == "" || binding.MeshSAEFile == "" || binding.Bridge == "" || binding.Network != "" || binding.Addressing != "" || binding.DefaultRoute || len(binding.VAPs) != 0 {
			return fmt.Errorf("startup contract mesh radio %q requires a mesh ID, SAE file, and bridge without IP addressing", binding.Name)
		}
		return nil
	}
	validateSecurity := func(security, path string) error {
		if binding.Band == "6ghz" && security != manifest.WirelessWPA3Personal {
			return fmt.Errorf("startup contract radio %q requires WPA3-Personal on 6 GHz", binding.Name)
		}
		switch security {
		case manifest.WirelessOpen:
			if path != "" {
				return fmt.Errorf("startup contract radio %q open profile cannot use a credential", binding.Name)
			}
		case manifest.WirelessWPA2Personal, manifest.WirelessWPA3Personal:
			if path == "" {
				return fmt.Errorf("startup contract radio %q secure profile requires a credential file", binding.Name)
			}
		default:
			return fmt.Errorf("startup contract radio %q has invalid security %q", binding.Name, security)
		}
		return nil
	}
	switch binding.Mode {
	case "ap":
		if binding.Network != "" || binding.Bridge != "" || binding.Addressing != "" || binding.DefaultRoute || len(binding.VAPs) < 1 || len(binding.VAPs) > 8 {
			return fmt.Errorf("startup contract AP radio %q requires 1 through 8 VAPs and no radio-level network, bridge, or addressing", binding.Name)
		}
		previousSlot := -1
		for _, vap := range binding.VAPs {
			if vap.Slot < 0 || vap.Slot > 7 || vap.Slot <= previousSlot || vap.Device == "" || vap.BSSID == "" || vap.Network == "" || vap.SSID == "" || vap.Bridge == "" {
				return fmt.Errorf("startup contract AP radio %q has invalid or unordered VAP slot %d", binding.Name, vap.Slot)
			}
			if err := validateSecurity(vap.Security, vap.PassphraseFile); err != nil {
				return err
			}
			previousSlot = vap.Slot
		}
		if binding.VAPs[0].Slot != 0 || binding.VAPs[0].Device != binding.Device || !strings.EqualFold(binding.VAPs[0].BSSID, binding.MAC) {
			return fmt.Errorf("startup contract AP radio %q slot 0 must use the primary radio identity", binding.Name)
		}
	case "station":
		if binding.Network == "" || binding.SSID == "" || binding.APBSSID == "" || binding.Addressing != "dhcp" || binding.Bridge != "" || len(binding.VAPs) != 0 {
			return fmt.Errorf("startup contract station radio %q requires one profile, DHCP, and no VAPs or bridge", binding.Name)
		}
		if err := validateSecurity(binding.Security, binding.PassphraseFile); err != nil {
			return err
		}
	default:
		return fmt.Errorf("startup contract radio %q mode must be ap or station", binding.Name)
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
		radios := make([]RadioBinding, 0, len(inst.Radios))
		for _, radio := range inst.Radios {
			medium := dep.WirelessNetwork(radio.Network)
			binding := RadioBinding{
				Name:         radio.Name,
				Device:       radio.Device,
				MAC:          radio.MAC,
				Mode:         radio.Mode,
				Network:      radio.Network,
				Bridge:       radio.Bridge,
				Addressing:   radio.Addressing,
				DefaultRoute: radio.DefaultRoute,
			}
			if radio.Mesh != nil {
				binding.MeshID = radio.Mesh.ID
				binding.MeshSAEFile = secrets.MeshCredentialContainerPath(radio.Mesh.ID)
			}
			if len(radio.Candidates) > 0 {
				binding.Candidates = append([]plan.RoamingCandidate(nil), radio.Candidates...)
			}
			if radio.Medium != "" {
				binding.Medium = radio.Medium
				binding.APBSSID = radio.APBSSID
				if rf := dep.WirelessMedium(radio.Medium); rf != nil {
					binding.Band, binding.Channel, binding.WidthMHz = rf.Band, rf.Channel, rf.WidthMHz
				}
				for _, vap := range radio.VAPs {
					resolved := VAPBinding{Slot: vap.Slot, Device: vap.Device, BSSID: vap.MAC, Network: vap.Network, Bridge: vap.Bridge}
					if profile := dep.WirelessNetwork(vap.Network); profile != nil {
						resolved.SSID, resolved.Security = profile.SSID, profile.Security
						if profile.PassphraseSecretRef != "" {
							resolved.PassphraseFile = secrets.WirelessCredentialContainerPath(profile.Name)
						}
					}
					binding.VAPs = append(binding.VAPs, resolved)
				}
				if medium != nil {
					binding.SSID, binding.Security = medium.SSID, medium.Security
					if medium.PassphraseSecretRef != "" {
						binding.PassphraseFile = secrets.WirelessCredentialContainerPath(medium.Name)
					}
				}
				radios = append(radios, binding)
				continue
			}
			if medium != nil {
				binding.SSID = medium.SSID
				binding.Channel = medium.Channel
				binding.Security = medium.Security
				binding.PassphraseSecretRef = medium.PassphraseSecretRef
				if len(binding.Candidates) > 0 && medium.PassphraseSecretRef != "" {
					binding.PassphraseFile = secrets.WirelessCredentialContainerPath(medium.Name)
					binding.PassphraseSecretRef = ""
				}
			}
			radios = append(radios, binding)
		}
		contracts[svc.Name] = Document{
			Version:    SupportedVersion,
			Service:    svc.Name,
			Deployment: dep.Name,
			Operation:  OperationContext{ID: opID},
			Interfaces: interfaces,
			Radios:     radios,
			Runtime:    RuntimeContext{ConfigPath: fmt.Sprintf("runtime/%s", svc.Name)},
		}
	}
	return contracts
}

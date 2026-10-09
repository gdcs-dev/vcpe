package app

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
)

type wirelessCredentialChanges struct {
	ForceRecreate   map[string]bool
	CreatedNetworks []string
	CreatedMeshIDs  []string
}

func prepareWirelessCredentials(stateRoot string, previous manifest.Document, hasPrevious bool, desired manifest.Document, deployment plan.Deployment, values map[string]string) (wirelessCredentialChanges, error) {
	changes := wirelessCredentialChanges{ForceRecreate: map[string]bool{}}
	previousNetworks := make(map[string]manifest.WirelessNetwork, len(previous.Spec.WirelessNetworks))
	for _, network := range previous.Spec.WirelessNetworks {
		previousNetworks[network.Name] = network
	}

	changedNetworks := map[string]bool{}
	for _, network := range desired.Spec.WirelessNetworks {
		prior, existed := previousNetworks[network.Name]
		policyChanged := !hasPrevious || !existed || prior.SSID != network.SSID || prior.Security != network.Security || prior.PassphraseSecretRef != network.PassphraseSecretRef
		if !isPersonalWireless(network.Security) {
			changedNetworks[network.Name] = policyChanged
			continue
		}

		credentialPath := secrets.WirelessCredentialPath(stateRoot, desired.Metadata.Name, network.Name)
		_, statErr := os.Lstat(credentialPath)
		missing := errors.Is(statErr, os.ErrNotExist)
		if statErr != nil && !missing {
			return changes, fmt.Errorf("inspect wireless credential for network %q: %w", network.Name, statErr)
		}
		result, err := secrets.MaterializeWirelessCredential(stateRoot, desired.Metadata.Name, network.Name, []byte(values[network.PassphraseSecretRef]))
		if err != nil {
			return changes, fmt.Errorf("materialize wireless credential for network %q: %w", network.Name, err)
		}
		if missing {
			changes.CreatedNetworks = append(changes.CreatedNetworks, network.Name)
		}
		changedNetworks[network.Name] = policyChanged || result.Changed
	}
	meshes := map[string]manifest.Mesh{}
	for _, service := range desired.Spec.Services {
		for _, radio := range service.Radios {
			if radio.Mesh != nil {
				meshes[radio.Mesh.ID] = *radio.Mesh
			}
		}
	}
	meshIDs := make([]string, 0, len(meshes))
	for meshID := range meshes {
		meshIDs = append(meshIDs, meshID)
	}
	sort.Strings(meshIDs)
	changedMeshes := map[string]bool{}
	for _, meshID := range meshIDs {
		path := secrets.MeshCredentialPath(stateRoot, desired.Metadata.Name, meshID)
		_, statErr := os.Lstat(path)
		missing := errors.Is(statErr, os.ErrNotExist)
		if statErr != nil && !missing {
			return changes, fmt.Errorf("inspect mesh credential %q: %w", meshID, statErr)
		}
		result, err := secrets.MaterializeMeshCredential(stateRoot, desired.Metadata.Name, meshID, []byte(values[meshes[meshID].SAESecretRef]))
		if err != nil {
			return changes, fmt.Errorf("materialize mesh credential %q: %w", meshID, err)
		}
		if missing {
			changes.CreatedMeshIDs = append(changes.CreatedMeshIDs, meshID)
		}
		changedMeshes[meshID] = result.Changed
	}

	previousServices := map[string]manifest.Service{}
	for _, service := range previous.Spec.Services {
		previousServices[service.Name] = service
	}
	desiredServices := map[string]manifest.Service{}
	for _, service := range desired.Spec.Services {
		desiredServices[service.Name] = service
	}
	for _, service := range deployment.Services {
		if hasPrevious && wirelessRadioPolicyChanged(previousServices[service.Name], desiredServices[service.Name]) {
			changes.ForceRecreate[service.Name] = true
		}
		for _, instance := range service.Instances {
			for _, radio := range instance.Radios {
				if radio.Mesh != nil && changedMeshes[radio.Mesh.ID] {
					changes.ForceRecreate[service.Name] = true
				}
				if changedNetworks[radio.Network] {
					changes.ForceRecreate[service.Name] = true
				}
				for _, vap := range radio.VAPs {
					if changedNetworks[vap.Network] {
						changes.ForceRecreate[service.Name] = true
					}
				}
			}
		}
	}
	return changes, nil
}

func wirelessRadioPolicyChanged(previous, desired manifest.Service) bool {
	previousRadios := map[string]manifest.Radio{}
	for _, radio := range previous.Radios {
		previousRadios[radio.Name] = radio
	}
	for _, radio := range desired.Radios {
		prior, exists := previousRadios[radio.Name]
		if radio.Mesh != nil && (!exists || !reflect.DeepEqual(prior.Mesh, radio.Mesh)) {
			return true
		}
		if radio.Medium == "" {
			continue
		}
		if !exists || prior.Network != radio.Network || len(prior.VAPs) != len(radio.VAPs) {
			return true
		}
		priorSlots := map[int]manifest.VAP{}
		for _, vap := range prior.VAPs {
			priorSlots[vap.Slot] = vap
		}
		for _, vap := range radio.VAPs {
			if priorSlots[vap.Slot] != vap {
				return true
			}
		}
	}
	return false
}

func isPersonalWireless(security string) bool {
	return security == manifest.WirelessWPA2Personal || security == manifest.WirelessWPA3Personal
}

func cleanupCreatedWirelessCredentials(stateRoot, deployment string, networks []string) error {
	var cleanupErrors []error
	for _, network := range networks {
		if err := secrets.RemoveWirelessCredential(stateRoot, deployment, network); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func cleanupCreatedMeshCredentials(stateRoot, deployment string, meshIDs []string) error {
	var cleanupErrors []error
	for _, meshID := range meshIDs {
		if err := secrets.RemoveMeshCredential(stateRoot, deployment, meshID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func desiredMeshIDs(document manifest.Document) []string {
	meshIDs := []string{}
	for _, service := range document.Spec.Services {
		for _, radio := range service.Radios {
			if radio.Mesh != nil {
				meshIDs = append(meshIDs, radio.Mesh.ID)
			}
		}
	}
	return meshIDs
}

func desiredPersonalWirelessNetworks(document manifest.Document) []string {
	networks := []string{}
	for _, network := range document.Spec.WirelessNetworks {
		if isPersonalWireless(network.Security) {
			networks = append(networks, network.Name)
		}
	}
	return networks
}

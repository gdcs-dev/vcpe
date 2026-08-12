package typeregistry

import "github.com/gdcs-dev/vcpe/controlplane/internal/manifest"

// BaseServiceType supplies the metadata defaults shared by curated service types.
type BaseServiceType struct{}

func (BaseServiceType) Health() HealthBehavior {
	return HealthBehavior{Mode: HealthModeCurated, ContainerPort: 9878}
}

func (BaseServiceType) DefaultImagePolicy() string { return "build" }

func (BaseServiceType) ValidateInterfaces(_ []manifest.Interface) error { return nil }

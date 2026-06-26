package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultRelativeStateRoot = ".local/state/vcpe-controlplane"
const artifactSchemaVersion = "v1"

func ResolveStateRoot(override string) (string, error) {
	if override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", fmt.Errorf("create state root: %w", err)
		}
		return override, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	stateRoot := filepath.Join(home, defaultRelativeStateRoot)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return "", fmt.Errorf("create state root: %w", err)
	}

	return stateRoot, nil
}

func ResolveSocketPath(stateRoot, override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(stateRoot, "vcpectl.sock")
}

func VersionedArtifactsRoot(stateRoot string) string {
	return filepath.Join(stateRoot, "artifacts", artifactSchemaVersion)
}

func OperationArtifactsDir(stateRoot, operationID string) string {
	return filepath.Join(VersionedArtifactsRoot(stateRoot), "operations", operationID)
}

func DeploymentArtifactsDir(stateRoot, customerID string) string {
	return filepath.Join(VersionedArtifactsRoot(stateRoot), "deployments", customerID)
}

func CustomerCompatibilityDir(stateRoot, customerID string) string {
	return filepath.Join(VersionedArtifactsRoot(stateRoot), "compat", customerID)
}

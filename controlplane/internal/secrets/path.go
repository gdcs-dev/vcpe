package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

const credentialSchemaVersion = "v1"

func RuntimeCredentialsRoot(stateRoot string) string {
	return filepath.Join(stateRoot, "credentials", credentialSchemaVersion)
}

func DeploymentCredentialsDir(stateRoot, deployment string) string {
	return filepath.Join(RuntimeCredentialsRoot(stateRoot), "deployments", pathComponent(deployment))
}

func WirelessCredentialPath(stateRoot, deployment, wirelessNetwork string) string {
	return filepath.Join(DeploymentCredentialsDir(stateRoot, deployment), "wireless", pathComponent(wirelessNetwork), "passphrase")
}

func WirelessCredentialContainerPath(wirelessNetwork string) string {
	return filepath.Join("/etc/vcpe/credentials/wireless", pathComponent(wirelessNetwork), "passphrase")
}

func MeshCredentialPath(stateRoot, deployment, meshID string) string {
	return filepath.Join(DeploymentCredentialsDir(stateRoot, deployment), "mesh", pathComponent(meshID), "passphrase")
}

func MeshCredentialContainerPath(meshID string) string {
	return filepath.Join("/etc/vcpe/credentials/mesh", pathComponent(meshID), "passphrase")
}

func pathComponent(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

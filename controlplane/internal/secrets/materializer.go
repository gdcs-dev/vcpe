package secrets

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type MaterializeResult struct {
	Path    string
	Changed bool
}

func MaterializeWirelessCredential(stateRoot, deployment, wirelessNetwork string, value []byte) (MaterializeResult, error) {
	target := WirelessCredentialPath(stateRoot, deployment, wirelessNetwork)
	return materializeCredential(stateRoot, target, value)
}

func MaterializeMeshCredential(stateRoot, deployment, meshID string, value []byte) (MaterializeResult, error) {
	target := MeshCredentialPath(stateRoot, deployment, meshID)
	return materializeCredential(stateRoot, target, value)
}

func materializeCredential(stateRoot, target string, value []byte) (MaterializeResult, error) {
	if err := ensureCredentialDirectories(stateRoot, filepath.Dir(target)); err != nil {
		return MaterializeResult{}, err
	}

	existing, err := readRegularCredential(target)
	if err != nil {
		return MaterializeResult{}, err
	}
	if existing != nil && bytes.Equal(existing, value) {
		if err := os.Chmod(target, 0o600); err != nil {
			return MaterializeResult{}, fmt.Errorf("set credential file mode: %w", err)
		}
		return MaterializeResult{Path: target}, nil
	}

	temporary, err := os.CreateTemp(filepath.Dir(target), ".passphrase-*")
	if err != nil {
		return MaterializeResult{}, fmt.Errorf("create temporary credential file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return MaterializeResult{}, fmt.Errorf("set temporary credential file mode: %w", err)
	}
	if _, err := temporary.Write(value); err != nil {
		cleanup()
		return MaterializeResult{}, fmt.Errorf("write temporary credential file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return MaterializeResult{}, fmt.Errorf("sync temporary credential file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return MaterializeResult{}, fmt.Errorf("close temporary credential file: %w", err)
	}
	if _, err := readRegularCredential(target); err != nil {
		cleanup()
		return MaterializeResult{}, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		cleanup()
		return MaterializeResult{}, fmt.Errorf("replace credential file: %w", err)
	}
	return MaterializeResult{Path: target, Changed: true}, nil
}

func ensureCredentialDirectories(stateRoot, targetDir string) error {
	directories := []string{
		filepath.Join(stateRoot, "credentials"),
		RuntimeCredentialsRoot(stateRoot),
		filepath.Join(RuntimeCredentialsRoot(stateRoot), "deployments"),
		filepath.Dir(filepath.Dir(targetDir)),
		filepath.Dir(targetDir),
		targetDir,
	}
	for _, directory := range directories {
		info, err := os.Lstat(directory)
		switch {
		case os.IsNotExist(err):
			if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create credential directory: %w", err)
			}
			info, err = os.Lstat(directory)
		case err != nil:
			return fmt.Errorf("inspect credential directory: %w", err)
		}
		if err != nil {
			return fmt.Errorf("inspect credential directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("credential path component is not a real directory")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("set credential directory mode: %w", err)
		}
	}
	return nil
}

func readRegularCredential(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect credential file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("credential path is not a regular file")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	return value, nil
}

func RemoveWirelessCredential(stateRoot, deployment, wirelessNetwork string) error {
	target := WirelessCredentialPath(stateRoot, deployment, wirelessNetwork)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove wireless credential: %w", err)
	}
	pruneEmptyCredentialDirectories(filepath.Dir(target), DeploymentCredentialsDir(stateRoot, deployment))
	return nil
}

func RemoveMeshCredential(stateRoot, deployment, meshID string) error {
	target := MeshCredentialPath(stateRoot, deployment, meshID)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove mesh credential: %w", err)
	}
	pruneEmptyCredentialDirectories(filepath.Dir(target), DeploymentCredentialsDir(stateRoot, deployment))
	return nil
}

func RemoveDeploymentCredentials(stateRoot, deployment string) error {
	directory := DeploymentCredentialsDir(stateRoot, deployment)
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("remove deployment credentials: %w", err)
	}
	pruneEmptyCredentialDirectories(filepath.Dir(directory), RuntimeCredentialsRoot(stateRoot))
	return nil
}

func RemoveAllCredentials(stateRoot string) error {
	if err := os.RemoveAll(RuntimeCredentialsRoot(stateRoot)); err != nil {
		return fmt.Errorf("remove runtime credentials: %w", err)
	}
	return nil
}

func RemoveObsoleteWirelessCredentials(stateRoot, deployment string, keepNetworks []string) error {
	wirelessDirectory := filepath.Join(DeploymentCredentialsDir(stateRoot, deployment), "wireless")
	entries, err := os.ReadDir(wirelessDirectory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list wireless credentials: %w", err)
	}
	keep := make(map[string]bool, len(keepNetworks))
	for _, network := range keepNetworks {
		keep[pathComponent(network)] = true
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(wirelessDirectory, entry.Name())); err != nil {
			return fmt.Errorf("remove obsolete wireless credential: %w", err)
		}
	}
	pruneEmptyCredentialDirectories(wirelessDirectory, DeploymentCredentialsDir(stateRoot, deployment))
	return nil
}

func RemoveObsoleteMeshCredentials(stateRoot, deployment string, keepIDs []string) error {
	meshDirectory := filepath.Join(DeploymentCredentialsDir(stateRoot, deployment), "mesh")
	entries, err := os.ReadDir(meshDirectory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list mesh credentials: %w", err)
	}
	keep := make(map[string]bool, len(keepIDs))
	for _, meshID := range keepIDs {
		keep[pathComponent(meshID)] = true
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(meshDirectory, entry.Name())); err != nil {
			return fmt.Errorf("remove obsolete mesh credential: %w", err)
		}
	}
	pruneEmptyCredentialDirectories(meshDirectory, DeploymentCredentialsDir(stateRoot, deployment))
	return nil
}

func RedactError(err error, values map[string]string) error {
	if err == nil {
		return nil
	}
	secretValues := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			secretValues = append(secretValues, value)
		}
	}
	sort.Slice(secretValues, func(i, j int) bool { return len(secretValues[i]) > len(secretValues[j]) })
	message := err.Error()
	for _, value := range secretValues {
		message = strings.ReplaceAll(message, value, "[REDACTED]")
	}
	return fmt.Errorf("%s", message)
}

func pruneEmptyCredentialDirectories(start, stop string) {
	for directory := start; directory != filepath.Dir(directory); directory = filepath.Dir(directory) {
		_ = os.Remove(directory)
		if directory == stop {
			return
		}
	}
}

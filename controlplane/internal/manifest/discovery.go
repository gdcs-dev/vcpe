package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry represents a manifest file discovered in the search path.
type Entry struct {
	Name        string // metadata.name from the manifest
	Path        string // absolute file path
	Description string // metadata.annotations.description, or "" if absent
}

// ErrNotFound is returned by Resolve when no manifest with the given name
// is found in the search directories.
var ErrNotFound = fmt.Errorf("manifest not found")

// header is the minimal YAML structure we parse during discovery — only the
// fields needed for identification and display. We deliberately do NOT parse
// spec (which can be large) to keep discovery fast.
type header struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name        string            `yaml:"name"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
}

// FindAll scans dirs in order and returns an Entry for every valid
// vcpe.dev/v1 Deployment manifest found. Invalid or unparseable files are
// silently skipped. Directories that do not exist are also silently skipped.
func FindAll(dirs []string) ([]Entry, error) {
	var entries []Entry
	seen := map[string]struct{}{} // deduplicate by absolute path

	for _, dir := range dirs {
		infos, err := os.ReadDir(dir)
		if err != nil {
			// Directory doesn't exist or isn't readable — skip.
			continue
		}
		for _, info := range infos {
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
				continue
			}
			abs := filepath.Join(dir, info.Name())
			if _, dup := seen[abs]; dup {
				continue
			}
			e, ok := readHeader(abs)
			if !ok {
				continue
			}
			seen[abs] = struct{}{}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// Resolve returns the absolute path of the first <name>.yaml found in dirs.
// Returns ("", ErrNotFound) when no match is found.
func Resolve(name string, dirs []string) (string, error) {
	target := name + ".yaml"
	for _, dir := range dirs {
		candidate := filepath.Join(dir, target)
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, nil
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("no manifest named %q found in search path: %w", name, ErrNotFound)
}

// SearchDirs returns the ordered list of directories to search for manifests.
// executableFn is injected (normally os.Executable) so callers and tests can
// control the anchor path without touching the real binary location.
//
// Order:
//  1. Colon-separated directories from VCPE_MANIFEST_DIRS (tilde-expanded)
//  2. <executableFn()>/../../share/vcpe/manifests/ (Homebrew pkgshare)
//     NOTE: symlinks are NOT resolved — the symlink path is used intentionally
//     so that $(brew --prefix)/bin/vcpe → ../../share/vcpe/manifests/ is correct.
//  3. ~/.vcpe/manifests/
//  4. ./manifests/ (CWD)
func SearchDirs(executableFn func() (string, error)) []string {
	var dirs []string

	// 1. VCPE_MANIFEST_DIRS
	if env := os.Getenv("VCPE_MANIFEST_DIRS"); env != "" {
		home, _ := os.UserHomeDir()
		for _, raw := range strings.Split(env, ":") {
			d := strings.TrimSpace(raw)
			if d == "" {
				continue
			}
			if strings.HasPrefix(d, "~/") && home != "" {
				d = filepath.Join(home, d[2:])
			}
			dirs = append(dirs, d)
		}
	}

	// 2. Homebrew pkgshare (relative to the raw executable path — no EvalSymlinks)
	// The binary lives at $(brew --prefix)/bin/vcpe; pkgshare is at
	// $(brew --prefix)/share/vcpe/manifests/ — one level up from bin/.
	if executableFn != nil {
		if exePath, err := executableFn(); err == nil {
			pkgshare := filepath.Join(filepath.Dir(exePath), "..", "share", "vcpe", "manifests")
			dirs = append(dirs, pkgshare)
		}
	}

	// 3. ~/.vcpe/manifests/
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".vcpe", "manifests"))
	}

	// 4. CWD ./manifests/
	dirs = append(dirs, filepath.Join(".", "manifests"))

	return dirs
}

// readHeader opens a YAML file, decodes just the header fields, and returns
// an Entry if the file is a valid vcpe.dev/v1 Deployment manifest.
func readHeader(path string) (Entry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Entry{}, false
	}
	defer f.Close()

	var h header
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&h); err != nil {
		return Entry{}, false
	}
	if h.APIVersion != APIVersion || h.Kind != Kind {
		return Entry{}, false
	}
	if h.Metadata.Name == "" {
		return Entry{}, false
	}

	desc := h.Metadata.Annotations["description"]
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return Entry{Name: h.Metadata.Name, Path: abs, Description: desc}, true
}

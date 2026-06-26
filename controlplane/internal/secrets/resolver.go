package secrets

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

func Resolve(refs []manifest.SecretRef) (map[string]string, error) {
	resolved := map[string]string{}
	for _, ref := range refs {
		var value string
		var err error
		switch ref.Provider {
		case "env":
			value, err = fromEnv(ref.Key)
		case "file":
			value, err = fromFile(ref.Key)
		default:
			err = fmt.Errorf("unsupported secret provider %q for ref %q", ref.Provider, ref.Name)
		}
		if err != nil {
			return nil, err
		}
		resolved[ref.Name] = value
	}
	return resolved, nil
}

func fromEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("missing secret ref env:%s", key)
	}
	return v, nil
}

func fromFile(key string) (string, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid file secret key %q: expected path:entry", key)
	}
	path := parts[0]
	entry := parts[1]

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open secret file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.TrimSpace(kv[0]) == entry {
			return strings.TrimSpace(kv[1]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read secret file %s: %w", path, err)
	}
	return "", fmt.Errorf("missing secret ref file:%s", entry)
}

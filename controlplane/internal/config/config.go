package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type FileConfig struct {
	StateRoot  string `json:"stateRoot" yaml:"stateRoot"`
	SocketPath string `json:"socketPath" yaml:"socketPath"`
	PolicyPath string `json:"policyPath" yaml:"policyPath"`
}

func Load(path string) (FileConfig, error) {
	if path == "" {
		return FileConfig{}, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, fmt.Errorf("read config: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var cfg FileConfig
	switch ext {
	case ".json":
		if err := json.Unmarshal(b, &cfg); err != nil {
			return FileConfig{}, fmt.Errorf("parse json config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return FileConfig{}, fmt.Errorf("parse yaml config: %w", err)
		}
	default:
		return FileConfig{}, errors.New("unsupported config extension: use .json, .yaml, or .yml")
	}

	return cfg, nil
}

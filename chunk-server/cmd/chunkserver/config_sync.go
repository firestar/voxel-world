package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"chunkserver/internal/config"
	"gopkg.in/yaml.v3"
)

func writeConfigFromCentral(cfgPath string) (bool, error) {
	jsonPayload := os.Getenv("CHUNK_CONFIG_JSON")
	yamlPayload := os.Getenv("CHUNK_CONFIG_YAML_B64")

	if jsonPayload == "" && yamlPayload == "" {
		return false, nil
	}
	if cfgPath == "" {
		return false, errors.New("central provided configuration but no --config path supplied")
	}

	var cfg config.Config
	if jsonPayload != "" {
		if err := json.Unmarshal([]byte(jsonPayload), &cfg); err != nil {
			return false, fmt.Errorf("decode central config json: %w", err)
		}
	} else {
		data, err := base64.StdEncoding.DecodeString(yamlPayload)
		if err != nil {
			return false, fmt.Errorf("decode central config yaml: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return false, fmt.Errorf("parse central config yaml: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return false, fmt.Errorf("validate central config: %w", err)
	}

	if cfgPath != "" {
		dir := filepath.Dir(cfgPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return false, fmt.Errorf("create config directory: %w", err)
			}
		}
		data, err := json.MarshalIndent(&cfg, "", "  ")
		if err != nil {
			return false, fmt.Errorf("marshal config json: %w", err)
		}
		if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
			return false, fmt.Errorf("write config file: %w", err)
		}
	}

	return true, nil
}

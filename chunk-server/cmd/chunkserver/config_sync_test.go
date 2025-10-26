package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"chunkserver/internal/config"
	"gopkg.in/yaml.v3"
)

func TestWriteConfigFromCentralJSON(t *testing.T) {
	t.Setenv("CHUNK_CONFIG_YAML_B64", "")

	cfg := config.Default()
	cfg.Server.ID = "json-config"
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	t.Setenv("CHUNK_CONFIG_JSON", string(data))

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	wrote, err := writeConfigFromCentral(path)
	if err != nil {
		t.Fatalf("writeConfigFromCentral: %v", err)
	}
	if !wrote {
		t.Fatalf("expected config to be written")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var decoded config.Config
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if decoded.Server.ID != "json-config" {
		t.Fatalf("unexpected server id: %q", decoded.Server.ID)
	}
}

func TestWriteConfigFromCentralYAML(t *testing.T) {
	cfg := config.Default()
	cfg.Server.ID = "yaml-config"
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal yaml: %v", err)
	}
	t.Setenv("CHUNK_CONFIG_JSON", "")
	t.Setenv("CHUNK_CONFIG_YAML_B64", base64.StdEncoding.EncodeToString(data))

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	wrote, err := writeConfigFromCentral(path)
	if err != nil {
		t.Fatalf("writeConfigFromCentral: %v", err)
	}
	if !wrote {
		t.Fatalf("expected config to be written")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var decoded config.Config
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if decoded.Server.ID != "yaml-config" {
		t.Fatalf("unexpected server id: %q", decoded.Server.ID)
	}
}

func TestWriteConfigFromCentralNoPayload(t *testing.T) {
	t.Setenv("CHUNK_CONFIG_JSON", "")
	t.Setenv("CHUNK_CONFIG_YAML_B64", "")

	wrote, err := writeConfigFromCentral("/tmp/unused.json")
	if err != nil {
		t.Fatalf("writeConfigFromCentral: %v", err)
	}
	if wrote {
		t.Fatalf("expected no config to be written")
	}
}

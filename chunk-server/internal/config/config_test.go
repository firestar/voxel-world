package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateDefaultConfig(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default configuration should be valid: %v", err)
	}
}

func TestValidateDetectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "missing server id",
			mutate: func(cfg *Config) {
				cfg.Server.ID = ""
			},
			wantErr: "server.id must be set",
		},
		{
			name: "non positive chunk dimensions",
			mutate: func(cfg *Config) {
				cfg.Chunk.Width = 0
			},
			wantErr: "chunk dimensions must be positive",
		},
		{
			name: "missing chunk per axis",
			mutate: func(cfg *Config) {
				cfg.Chunk.ChunksPerAxis = 0
			},
			wantErr: "chunk.chunksPerAxis must be positive",
		},
		{
			name: "missing network listen address",
			mutate: func(cfg *Config) {
				cfg.Network.ListenUDP = ""
			},
			wantErr: "network.listenUdp must be set",
		},
		{
			name: "non positive max entities",
			mutate: func(cfg *Config) {
				cfg.Entities.MaxEntitiesPerChunk = 0
			},
			wantErr: "entities.maxEntitiesPerChunk must be positive",
		},
		{
			name: "negative movement workers",
			mutate: func(cfg *Config) {
				cfg.Entities.MovementWorkers = -1
			},
			wantErr: "entities.movementWorkers cannot be negative",
		},
		{
			name: "negative terrain workers",
			mutate: func(cfg *Config) {
				cfg.Terrain.Workers = -1
			},
			wantErr: "terrain.workers cannot be negative",
		},
		{
			name: "missing block id",
			mutate: func(cfg *Config) {
				cfg.Blocks[0].ID = ""
			},
			wantErr: "blocks[0].id must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("unexpected error: got %q want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadEmptyPathReturnsDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if want := Default(); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("default configuration mismatch:\nwant: %#v\n got: %#v", want, cfg)
	}
}

func TestLoadReadsFileAndValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.Server.Description = "custom description"
	cfg.Network.ListenUDP = ":9999"

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("loaded configuration mismatch:\nwant: %#v\n got: %#v", cfg, got)
	}
}

func TestLoadInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.Chunk.Width = 0

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err = Load(path)
	if err == nil {
		t.Fatalf("expected load to fail")
	}
	if !strings.Contains(err.Error(), "validate config: chunk dimensions must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

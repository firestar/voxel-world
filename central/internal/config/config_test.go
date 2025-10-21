package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAppliesDefaultsAndClusterExecutable(t *testing.T) {
	cfg := &Config{
		ChunkServers: []ChunkServer{{
			ID:        "alpha",
			ChunkSpan: ChunkSpan{ChunksX: 1, ChunksY: 1},
		}},
		Cluster: ClusterConfig{DefaultBinary: "/usr/bin/chunk"},
		World:   WorldConfig{ChunkWidth: 16, ChunkDepth: 16, ChunkHeight: 256},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}

	if got := cfg.ListenAddress; got != "0.0.0.0" {
		t.Fatalf("ListenAddress = %q, want %q", got, "0.0.0.0")
	}
	if got := cfg.HTTPPort; got != 28080 {
		t.Fatalf("HTTPPort = %d, want %d", got, 28080)
	}
	if got := cfg.ChunkServers[0].Executable; got != "/usr/bin/chunk" {
		t.Fatalf("Executable = %q, want %q", got, "/usr/bin/chunk")
	}
}

func TestValidateRejectsInvalidConfigurations(t *testing.T) {
	validWorld := WorldConfig{ChunkWidth: 16, ChunkDepth: 16, ChunkHeight: 256}
	tests := map[string]*Config{
		"no chunk servers": {
			World: validWorld,
		},
		"missing world dims": {
			ChunkServers: []ChunkServer{{
				ID:         "alpha",
				ChunkSpan:  ChunkSpan{ChunksX: 1, ChunksY: 1},
				Executable: "/bin/true",
			}},
		},
		"missing id": {
			ChunkServers: []ChunkServer{{
				ChunkSpan:  ChunkSpan{ChunksX: 1, ChunksY: 1},
				Executable: "/bin/true",
			}},
			World: validWorld,
		},
		"non-positive span": {
			ChunkServers: []ChunkServer{{
				ID:         "alpha",
				ChunkSpan:  ChunkSpan{ChunksX: 0, ChunksY: 1},
				Executable: "/bin/true",
			}},
			World: validWorld,
		},
		"missing executable without default": {
			ChunkServers: []ChunkServer{{
				ID:        "alpha",
				ChunkSpan: ChunkSpan{ChunksX: 1, ChunksY: 1},
			}},
			World: validWorld,
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
		})
	}
}

func TestLoadReadsYAMLAndValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
listen_address: ""
http_port: 0
cluster:
  default_binary: "/usr/bin/chunk"
chunk_servers:
  - id: "alpha"
    chunk_span:
      chunks_x: 1
      chunks_y: 1
world:
  chunk_width: 16
  chunk_depth: 16
  chunk_height: 256
`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.ListenAddress != "0.0.0.0" {
		t.Errorf("ListenAddress = %q, want 0.0.0.0", cfg.ListenAddress)
	}
	if cfg.HTTPPort != 28080 {
		t.Errorf("HTTPPort = %d, want 28080", cfg.HTTPPort)
	}
	if cfg.ChunkServers[0].Executable != "/usr/bin/chunk" {
		t.Errorf("Executable = %q, want /usr/bin/chunk", cfg.ChunkServers[0].Executable)
	}
}

func TestLoadPropagatesReadErrors(t *testing.T) {
	if _, err := Load("/nonexistent/path.yaml"); err == nil {
		t.Fatalf("Load() = nil, want error")
	}
}

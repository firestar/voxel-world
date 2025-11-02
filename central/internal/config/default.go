package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Default returns a configuration populated with sensible defaults so that a
// central server can be started without any prior configuration.
func Default() Config {
	return Config{
		ListenAddress: "0.0.0.0",
		HTTPPort:      28080,
		World: WorldConfig{
			ChunkWidth:  512,
			ChunkDepth:  512,
			ChunkHeight: 2048,
			DayLength:   "20m",
			InitialHour: 12.0,
			Blocks:      DefaultBlocks(),
		},
		Cluster: ClusterConfig{
			DefaultBinary: "../chunk-server/cmd/chunkserver/chunkserver",
			DataRoot:      "./data",
			Env: map[string]string{
				"CHUNK_LOG_LEVEL": "INFO",
			},
		},
		ChunkServers: []ChunkServer{
			{
				ID: "chunk-east-0",
				GlobalOrigin: ChunkOrigin{
					ChunkX: 0,
					ChunkY: 0,
				},
				ChunkSpan: ChunkSpan{
					ChunksX: 32,
					ChunksY: 32,
				},
				Args:          []string{"--config", "configs/chunk-east.json"},
				ListenAddress: "127.0.0.1:19000",
				HttpAddress:   "http://127.0.0.1:19001",
			},
			{
				ID: "chunk-west-0",
				GlobalOrigin: ChunkOrigin{
					ChunkX: 32,
					ChunkY: 0,
				},
				ChunkSpan: ChunkSpan{
					ChunksX: 32,
					ChunksY: 32,
				},
				Args:          []string{"--config", "configs/chunk-west.json"},
				ListenAddress: "127.0.0.1:19100",
				HttpAddress:   "http://127.0.0.1:19101",
			},
		},
		PlayerAPI: PlayerAPIConfig{
			Enabled: true,
			BaseURL: "https://api.example.com",
		},
	}
}

// WriteDefault writes the default configuration to the provided path.
func WriteDefault(path string) error {
	cfg := Default()

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}

	return nil
}

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddress string          `yaml:"listen_address"`
	HTTPPort      int             `yaml:"http_port"`
	Cluster       ClusterConfig   `yaml:"cluster"`
	ChunkServers  []ChunkServer   `yaml:"chunk_servers"`
	PlayerAPI     PlayerAPIConfig `yaml:"player_api"`
	World         WorldConfig     `yaml:"world"`
}

type ClusterConfig struct {
	DefaultBinary string            `yaml:"default_binary"`
	DataRoot      string            `yaml:"data_root"`
	Env           map[string]string `yaml:"env"`
}

type ChunkServer struct {
	ID             string            `yaml:"id"`
	GlobalOrigin   ChunkOrigin       `yaml:"global_origin"`
	ChunkSpan      ChunkSpan         `yaml:"chunk_span"`
	Executable     string            `yaml:"executable"`
	ContainerImage string            `yaml:"container_image"`
	Args           []string          `yaml:"args"`
	Env            map[string]string `yaml:"env"`
	ListenAddress  string            `yaml:"listen_address"`
	HttpAddress    string            `yaml:"http_address"`
}

type ChunkOrigin struct {
	ChunkX int `yaml:"chunk_x"`
	ChunkY int `yaml:"chunk_y"`
}

type ChunkSpan struct {
	ChunksX int `yaml:"chunks_x"`
	ChunksY int `yaml:"chunks_y"`
}

type PlayerAPIConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
}

type WorldConfig struct {
	ChunkWidth  int `yaml:"chunk_width"`
	ChunkDepth  int `yaml:"chunk_depth"`
	ChunkHeight int `yaml:"chunk_height"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.ListenAddress == "" {
		c.ListenAddress = "0.0.0.0"
	}
	if c.HTTPPort == 0 {
		c.HTTPPort = 28080
	}
	if len(c.ChunkServers) == 0 {
		return fmt.Errorf("chunk_servers cannot be empty")
	}
	if c.World.ChunkWidth <= 0 || c.World.ChunkDepth <= 0 || c.World.ChunkHeight <= 0 {
		return fmt.Errorf("world chunk dimensions must be positive")
	}
	for i, cs := range c.ChunkServers {
		if cs.ID == "" {
			return fmt.Errorf("chunk_servers[%d].id must be set", i)
		}
		if cs.ChunkSpan.ChunksX <= 0 || cs.ChunkSpan.ChunksY <= 0 {
			return fmt.Errorf("chunk_servers[%d] chunk span must be positive", i)
		}
		if cs.Executable == "" && cs.ContainerImage == "" {
			if c.Cluster.DefaultBinary == "" {
				return fmt.Errorf("chunk_servers[%d].executable empty and no cluster.default_binary provided", i)
			}
			c.ChunkServers[i].Executable = c.Cluster.DefaultBinary
		}
	}
	return nil
}

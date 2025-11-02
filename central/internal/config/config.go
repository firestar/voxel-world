package config

import (
	"fmt"
	"os"
	"time"

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
	ChunkWidth  int               `yaml:"chunk_width"`
	ChunkDepth  int               `yaml:"chunk_depth"`
	ChunkHeight int               `yaml:"chunk_height"`
	Blocks      []BlockDefinition `yaml:"blocks"`
	DayLength   string            `yaml:"day_length"`
	InitialHour float64           `yaml:"initial_hour"`
}

type BlockDefinition struct {
	ID            string           `yaml:"id" json:"id"`
	Color         string           `yaml:"color" json:"color"`
	Spawn         BlockSpawnConfig `yaml:"spawn" json:"spawn"`
	LightEmission float64          `yaml:"light_emission" json:"lightEmission"`
}

type BlockSpawnConfig struct {
	Type        string `yaml:"type" json:"type"`
	VeinSizeMin int    `yaml:"vein_size_min,omitempty" json:"veinSizeMin,omitempty"`
	VeinSizeMax int    `yaml:"vein_size_max,omitempty" json:"veinSizeMax,omitempty"`
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
	if c.World.DayLength == "" {
		c.World.DayLength = "20m"
	}
	if _, err := time.ParseDuration(c.World.DayLength); err != nil {
		return fmt.Errorf("world.day_length invalid: %w", err)
	}
	if c.World.InitialHour < 0 || c.World.InitialHour >= 24 {
		c.World.InitialHour = 12.0
	}
	if err := validateWorldBlocks(c.World.Blocks); err != nil {
		return err
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

func validateWorldBlocks(blocks []BlockDefinition) error {
	if len(blocks) == 0 {
		return fmt.Errorf("world.blocks cannot be empty")
	}
	for i, block := range blocks {
		if block.ID == "" {
			return fmt.Errorf("world.blocks[%d].id must be set", i)
		}
		if !isValidHexColor(block.Color) {
			return fmt.Errorf("world.blocks[%d].color must be a hex RGB value", i)
		}
		if block.LightEmission < 0 {
			return fmt.Errorf("world.blocks[%d].light_emission cannot be negative", i)
		}
		switch block.Spawn.Type {
		case "solo":
			if block.Spawn.VeinSizeMin != 0 || block.Spawn.VeinSizeMax != 0 {
				return fmt.Errorf("world.blocks[%d].spawn vein sizes must be zero for solo blocks", i)
			}
		case "vein":
			if block.Spawn.VeinSizeMin <= 0 || block.Spawn.VeinSizeMax <= 0 {
				return fmt.Errorf("world.blocks[%d].spawn vein sizes must be positive", i)
			}
			if block.Spawn.VeinSizeMin > block.Spawn.VeinSizeMax {
				return fmt.Errorf("world.blocks[%d].spawn vein_size_min cannot exceed vein_size_max", i)
			}
		default:
			return fmt.Errorf("world.blocks[%d].spawn.type must be either 'solo' or 'vein'", i)
		}
	}
	return nil
}

func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, ch := range s[1:] {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		case ch >= 'A' && ch <= 'F':
		default:
			return false
		}
	}
	return true
}

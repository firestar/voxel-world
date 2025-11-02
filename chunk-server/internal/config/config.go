package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Duration is a JSON-friendly wrapper around time.Duration that accepts human
// readable strings such as "150ms" in configuration files while still
// allowing numeric representations when necessary.
type Duration time.Duration

// Duration returns the underlying time.Duration value.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// MarshalJSON encodes the duration using the canonical string representation.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON decodes a duration from either a string (e.g. "250ms") or a
// numeric value representing nanoseconds. Empty strings and null values decode
// to zero.
func (d *Duration) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("duration: empty value")
	}
	if string(b) == "null" {
		*d = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("duration: decode string: %w", err)
		}
		if s == "" {
			*d = 0
			return nil
		}
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("duration: parse %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		*d = Duration(time.Duration(n))
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*d = Duration(time.Duration(f))
		return nil
	}
	return fmt.Errorf("duration: invalid value %s", string(b))
}

// Config captures the tunable parameters needed to bootstrap a chunk server.
type Config struct {
	Server      ServerConfig      `json:"server"`
	Chunk       ChunkConfig       `json:"chunk"`
	Network     NetworkConfig     `json:"network"`
	Pathfinding PathfindingConfig `json:"pathfinding"`
	Terrain     TerrainConfig     `json:"terrain"`
	Economy     EconomyConfig     `json:"economy"`
	Entities    EntityConfig      `json:"entities"`
	Environment EnvironmentConfig `json:"environment"`
	Blocks      []BlockDefinition `json:"blocks"`
}

type ServerConfig struct {
	ID                 string     `json:"id"`
	Description        string     `json:"description"`
	GlobalChunkOrigin  ChunkIndex `json:"globalChunkOrigin"`
	TickRate           Duration   `json:"tickRate"`           // e.g. "33ms"
	StateStreamRate    Duration   `json:"stateStreamRate"`    // frequency at which deltas are broadcast
	EntityStreamRate   Duration   `json:"entityStreamRate"`   // frequency for entity refreshes
	MaxConcurrentLoads int        `json:"maxConcurrentLoads"` // simultaneous chunk mesh/generation jobs
}

type ChunkConfig struct {
	Width         int `json:"width"`
	Depth         int `json:"depth"`
	Height        int `json:"height"`
	ChunksPerAxis int `json:"chunksPerAxis"`
}

type NetworkConfig struct {
	ListenUDP            string        `json:"listenUdp"`            // ":9000"
	MainServerEndpoints  []string      `json:"mainServerEndpoints"`  // list of UDP endpoints to stream to
	NeighborEndpoints    []NeighborRef `json:"neighborEndpoints"`    // optional explicit neighbor override
	HandshakeTimeout     Duration      `json:"handshakeTimeout"`     // e.g. "3s"
	KeepAliveInterval    Duration      `json:"keepAliveInterval"`    // periodic keep alive ping
	MaxDatagramSizeBytes int           `json:"maxDatagramSizeBytes"` // default to 64 KiB - UDP practical limit
	DiscoveryInterval    Duration      `json:"discoveryInterval"`    // how often to query for neighbors
	TransferRetry        Duration      `json:"transferRetry"`        // back-off for failed chunk transfers
}

type NeighborRef struct {
	ChunkDelta ChunkIndex `json:"chunkDelta"` // relative offset from this server's origin
	Endpoint   string     `json:"endpoint"`
}

type PathfindingConfig struct {
	MaxSearchNodes    int      `json:"maxSearchNodes"`
	HeuristicScale    float64  `json:"heuristicScale"`
	AsyncWorkers      int      `json:"asyncWorkers"`
	ThrottlePerSecond int      `json:"throttlePerSecond"`
	QueueTimeout      Duration `json:"queueTimeout"`
}

type TerrainConfig struct {
	Seed        int64   `json:"seed"`
	Frequency   float64 `json:"frequency"`
	Amplitude   float64 `json:"amplitude"`
	Octaves     int     `json:"octaves"`
	Persistence float64 `json:"persistence"`
	Lacunarity  float64 `json:"lacunarity"`
}

type EconomyConfig struct {
	ResourceSpawnDensity map[string]float64 `json:"resourceSpawnDensity"`
	MiningLevelGrowth    float64            `json:"miningLevelGrowth"` // multiplier per miner level
	BaseMiningRate       float64            `json:"baseMiningRate"`    // blocks per second
}

type EntityConfig struct {
	MaxEntitiesPerChunk int      `json:"maxEntitiesPerChunk"`
	EntityTickRate      Duration `json:"entityTickRate"`
	ProjectileTickRate  Duration `json:"projectileTickRate"`
	MovementWorkers     int      `json:"movementWorkers"`
}

type EnvironmentConfig struct {
	DayLength          Duration `json:"dayLength"`
	WeatherMinDuration Duration `json:"weatherMinDuration"`
	WeatherMaxDuration Duration `json:"weatherMaxDuration"`
	StormChance        float64  `json:"stormChance"`
	RainChance         float64  `json:"rainChance"`
	WindBase           float64  `json:"windBase"`
	WindVariance       float64  `json:"windVariance"`
	Seed               int64    `json:"seed"`
}

type ChunkIndex struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type BlockDefinition struct {
	ID            string           `json:"id"`
	Color         string           `json:"color"`
	Spawn         BlockSpawnConfig `json:"spawn"`
	LightEmission float64          `json:"lightEmission"`
}

type BlockSpawnConfig struct {
	Type        string `json:"type"`
	VeinSizeMin int    `json:"veinSizeMin,omitempty"`
	VeinSizeMax int    `json:"veinSizeMax,omitempty"`
}

// Load reads configuration from a JSON file if provided. An empty path returns defaults.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			ID:                 "chunk-server-0",
			Description:        "local development chunk server",
			GlobalChunkOrigin:  ChunkIndex{X: 0, Y: 0},
			TickRate:           Duration(33 * time.Millisecond),
			StateStreamRate:    Duration(200 * time.Millisecond),
			EntityStreamRate:   Duration(50 * time.Millisecond),
			MaxConcurrentLoads: 4,
		},
		Chunk: ChunkConfig{
			Width:         512,
			Depth:         512,
			Height:        2048,
			ChunksPerAxis: 32,
		},
		Network: NetworkConfig{
			ListenUDP:            ":19000",
			MainServerEndpoints:  []string{"127.0.0.1:20000"},
			NeighborEndpoints:    []NeighborRef{},
			HandshakeTimeout:     Duration(3 * time.Second),
			KeepAliveInterval:    Duration(5 * time.Second),
			MaxDatagramSizeBytes: 1 << 16,
			DiscoveryInterval:    Duration(10 * time.Second),
			TransferRetry:        Duration(2 * time.Second),
		},
		Pathfinding: PathfindingConfig{
			MaxSearchNodes:    50_000,
			HeuristicScale:    1.0,
			AsyncWorkers:      4,
			ThrottlePerSecond: 120,
			QueueTimeout:      Duration(250 * time.Millisecond),
		},
		Terrain: TerrainConfig{
			Seed:        1337,
			Frequency:   0.003,
			Amplitude:   512,
			Octaves:     4,
			Persistence: 0.45,
			Lacunarity:  2.0,
		},
		Economy: EconomyConfig{
			ResourceSpawnDensity: map[string]float64{
				"steel":       0.9,
				"uranium":     0.25,
				"plastanium":  0.4,
				"vibranium":   0.1,
				"electronium": 0.15,
				"foodium":     0.6,
			},
			MiningLevelGrowth: 1.15,
			BaseMiningRate:    3.0,
		},
		Entities: EntityConfig{
			MaxEntitiesPerChunk: 4096,
			EntityTickRate:      Duration(33 * time.Millisecond),
			ProjectileTickRate:  Duration(16 * time.Millisecond),
			MovementWorkers:     1,
		},
		Environment: EnvironmentConfig{
			DayLength:          Duration(20 * time.Minute),
			WeatherMinDuration: Duration(2 * time.Minute),
			WeatherMaxDuration: Duration(5 * time.Minute),
			StormChance:        0.15,
			RainChance:         0.35,
			WindBase:           3.0,
			WindVariance:       5.0,
			Seed:               1337,
		},
		Blocks: defaultBlockDefinitions(),
	}
}

func (c *Config) Validate() error {
	if c.Server.ID == "" {
		return errors.New("server.id must be set")
	}
	if c.Chunk.Width <= 0 || c.Chunk.Depth <= 0 || c.Chunk.Height <= 0 {
		return errors.New("chunk dimensions must be positive")
	}
	if c.Chunk.ChunksPerAxis <= 0 {
		return errors.New("chunk.chunksPerAxis must be positive")
	}
	if c.Network.ListenUDP == "" {
		return errors.New("network.listenUdp must be set")
	}
	if c.Entities.MaxEntitiesPerChunk <= 0 {
		return errors.New("entities.maxEntitiesPerChunk must be positive")
	}
	if c.Entities.MovementWorkers < 0 {
		return errors.New("entities.movementWorkers cannot be negative")
	}
	if c.Environment.WeatherMaxDuration > 0 && c.Environment.WeatherMaxDuration < c.Environment.WeatherMinDuration {
		return errors.New("environment.weatherMaxDuration must be >= weatherMinDuration")
	}
	if c.Environment.StormChance < 0 || c.Environment.RainChance < 0 {
		return errors.New("environment storm/rain chances cannot be negative")
	}
	if c.Environment.StormChance+c.Environment.RainChance > 1.0 {
		return errors.New("environment storm+rain chance must be <= 1")
	}
	if err := validateBlocks(c.Blocks); err != nil {
		return err
	}
	return nil
}

func validateBlocks(blocks []BlockDefinition) error {
	if len(blocks) == 0 {
		return errors.New("blocks cannot be empty")
	}
	for i, block := range blocks {
		if block.ID == "" {
			return fmt.Errorf("blocks[%d].id must be set", i)
		}
		if !isValidHexColor(block.Color) {
			return fmt.Errorf("blocks[%d].color must be a hex RGB value", i)
		}
		if block.LightEmission < 0 {
			return fmt.Errorf("blocks[%d].lightEmission cannot be negative", i)
		}
		switch block.Spawn.Type {
		case "solo":
			if block.Spawn.VeinSizeMin != 0 || block.Spawn.VeinSizeMax != 0 {
				return fmt.Errorf("blocks[%d].spawn vein sizes must be zero for solo blocks", i)
			}
		case "vein":
			if block.Spawn.VeinSizeMin <= 0 || block.Spawn.VeinSizeMax <= 0 {
				return fmt.Errorf("blocks[%d].spawn vein sizes must be positive", i)
			}
			if block.Spawn.VeinSizeMin > block.Spawn.VeinSizeMax {
				return fmt.Errorf("blocks[%d].spawn veinSizeMin cannot exceed veinSizeMax", i)
			}
		default:
			return fmt.Errorf("blocks[%d].spawn.type must be either 'solo' or 'vein'", i)
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

// defaultBlockDefinitions returns the default block set used when no explicit
// configuration is provided. Keep in sync with central/internal/config.
func defaultBlockDefinitions() []BlockDefinition {
	return []BlockDefinition{
		{ID: "dirt", Color: "#8B5A2B", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 32, VeinSizeMax: 96}},
		{ID: "sand", Color: "#C2B280", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 24, VeinSizeMax: 80}},
		{ID: "slate", Color: "#2F4F4F", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 16, VeinSizeMax: 48}},
		{ID: "sandstone", Color: "#D2B48C", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 20, VeinSizeMax: 64}},
		{ID: "obsidian", Color: "#341A34", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 8, VeinSizeMax: 20}},
		{ID: "shale", Color: "#4B3F32", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 16, VeinSizeMax: 40}},
		{ID: "cobblestone", Color: "#8A8A8A", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 24, VeinSizeMax: 70}},
		{ID: "coal", Color: "#2B2B2B", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 6, VeinSizeMax: 18}},
		{ID: "oil_soaked_rock", Color: "#3B2F2F", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 12}},
		{ID: "iron", Color: "#B7410E", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 12}},
		{ID: "copper", Color: "#B87333", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 14}},
		{ID: "gold", Color: "#FFD700", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 3, VeinSizeMax: 8}},
		{ID: "silver", Color: "#C0C0C0", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 3, VeinSizeMax: 8}},
		{ID: "uranium", Color: "#6B8E23", Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 2, VeinSizeMax: 5}},
		{ID: "unobtainium", Color: "#7F00FF", Spawn: BlockSpawnConfig{Type: "solo"}},
		{ID: "unit_lumen_panel", Color: "#FFF3B0", Spawn: BlockSpawnConfig{Type: "solo"}, LightEmission: 6.0},
		{ID: "structure_arc_lamp", Color: "#FFE066", Spawn: BlockSpawnConfig{Type: "solo"}, LightEmission: 10.0},
	}
}

package cluster

import "central/internal/config"

type chunkServerConfig struct {
	Server      chunkServerServerConfig      `json:"server" yaml:"server"`
	Chunk       chunkServerChunkConfig       `json:"chunk" yaml:"chunk"`
	Network     chunkServerNetworkConfig     `json:"network" yaml:"network"`
	Pathfinding chunkServerPathfindingConfig `json:"pathfinding" yaml:"pathfinding"`
	Terrain     chunkServerTerrainConfig     `json:"terrain" yaml:"terrain"`
	Economy     chunkServerEconomyConfig     `json:"economy" yaml:"economy"`
	Entities    chunkServerEntitiesConfig    `json:"entities" yaml:"entities"`
	Environment chunkServerEnvironmentConfig `json:"environment" yaml:"environment"`
}

type chunkServerServerConfig struct {
	ID                 string              `json:"id" yaml:"id"`
	Description        string              `json:"description" yaml:"description"`
	GlobalChunkOrigin  chunkServerChunkRef `json:"globalChunkOrigin" yaml:"globalChunkOrigin"`
	TickRate           string              `json:"tickRate" yaml:"tickRate"`
	StateStreamRate    string              `json:"stateStreamRate" yaml:"stateStreamRate"`
	EntityStreamRate   string              `json:"entityStreamRate" yaml:"entityStreamRate"`
	MaxConcurrentLoads int                 `json:"maxConcurrentLoads" yaml:"maxConcurrentLoads"`
}

type chunkServerChunkConfig struct {
	Width         int `json:"width" yaml:"width"`
	Depth         int `json:"depth" yaml:"depth"`
	Height        int `json:"height" yaml:"height"`
	ChunksPerAxis int `json:"chunksPerAxis" yaml:"chunksPerAxis"`
}

type chunkServerNetworkConfig struct {
	ListenUDP            string                   `json:"listenUdp" yaml:"listenUdp"`
	MainServerEndpoints  []string                 `json:"mainServerEndpoints" yaml:"mainServerEndpoints"`
	NeighborEndpoints    []chunkServerNeighborRef `json:"neighborEndpoints" yaml:"neighborEndpoints"`
	HandshakeTimeout     string                   `json:"handshakeTimeout" yaml:"handshakeTimeout"`
	KeepAliveInterval    string                   `json:"keepAliveInterval" yaml:"keepAliveInterval"`
	MaxDatagramSizeBytes int                      `json:"maxDatagramSizeBytes" yaml:"maxDatagramSizeBytes"`
	DiscoveryInterval    string                   `json:"discoveryInterval" yaml:"discoveryInterval"`
	TransferRetry        string                   `json:"transferRetry" yaml:"transferRetry"`
}

type chunkServerNeighborRef struct {
	ChunkDelta chunkServerChunkRef `json:"chunkDelta" yaml:"chunkDelta"`
	Endpoint   string              `json:"endpoint" yaml:"endpoint"`
}

type chunkServerPathfindingConfig struct {
	MaxSearchNodes    int     `json:"maxSearchNodes" yaml:"maxSearchNodes"`
	HeuristicScale    float64 `json:"heuristicScale" yaml:"heuristicScale"`
	AsyncWorkers      int     `json:"asyncWorkers" yaml:"asyncWorkers"`
	ThrottlePerSecond int     `json:"throttlePerSecond" yaml:"throttlePerSecond"`
	QueueTimeout      string  `json:"queueTimeout" yaml:"queueTimeout"`
}

type chunkServerTerrainConfig struct {
	Seed        int64   `json:"seed" yaml:"seed"`
	Frequency   float64 `json:"frequency" yaml:"frequency"`
	Amplitude   float64 `json:"amplitude" yaml:"amplitude"`
	Octaves     int     `json:"octaves" yaml:"octaves"`
	Persistence float64 `json:"persistence" yaml:"persistence"`
	Lacunarity  float64 `json:"lacunarity" yaml:"lacunarity"`
}

type chunkServerEconomyConfig struct {
	ResourceSpawnDensity map[string]float64 `json:"resourceSpawnDensity" yaml:"resourceSpawnDensity"`
	MiningLevelGrowth    float64            `json:"miningLevelGrowth" yaml:"miningLevelGrowth"`
	BaseMiningRate       float64            `json:"baseMiningRate" yaml:"baseMiningRate"`
}

type chunkServerEntitiesConfig struct {
	MaxEntitiesPerChunk int    `json:"maxEntitiesPerChunk" yaml:"maxEntitiesPerChunk"`
	EntityTickRate      string `json:"entityTickRate" yaml:"entityTickRate"`
	ProjectileTickRate  string `json:"projectileTickRate" yaml:"projectileTickRate"`
	MovementWorkers     int    `json:"movementWorkers" yaml:"movementWorkers"`
}

type chunkServerEnvironmentConfig struct {
	DayLength          string  `json:"dayLength" yaml:"dayLength"`
	WeatherMinDuration string  `json:"weatherMinDuration" yaml:"weatherMinDuration"`
	WeatherMaxDuration string  `json:"weatherMaxDuration" yaml:"weatherMaxDuration"`
	StormChance        float64 `json:"stormChance" yaml:"stormChance"`
	RainChance         float64 `json:"rainChance" yaml:"rainChance"`
	WindBase           float64 `json:"windBase" yaml:"windBase"`
	WindVariance       float64 `json:"windVariance" yaml:"windVariance"`
	Seed               int64   `json:"seed" yaml:"seed"`
}

type chunkServerChunkRef struct {
	X int `json:"x" yaml:"x"`
	Y int `json:"y" yaml:"y"`
}

func defaultChunkServerConfig() chunkServerConfig {
	return chunkServerConfig{
		Server: chunkServerServerConfig{
			ID:                 "chunk-server-0",
			Description:        "local development chunk server",
			GlobalChunkOrigin:  chunkServerChunkRef{X: 0, Y: 0},
			TickRate:           "33ms",
			StateStreamRate:    "200ms",
			EntityStreamRate:   "50ms",
			MaxConcurrentLoads: 4,
		},
		Chunk: chunkServerChunkConfig{
			Width:         512,
			Depth:         512,
			Height:        2048,
			ChunksPerAxis: 32,
		},
		Network: chunkServerNetworkConfig{
			ListenUDP:            ":19000",
			MainServerEndpoints:  []string{"127.0.0.1:20000"},
			NeighborEndpoints:    []chunkServerNeighborRef{},
			HandshakeTimeout:     "3s",
			KeepAliveInterval:    "5s",
			MaxDatagramSizeBytes: 1 << 16,
			DiscoveryInterval:    "10s",
			TransferRetry:        "2s",
		},
		Pathfinding: chunkServerPathfindingConfig{
			MaxSearchNodes:    50_000,
			HeuristicScale:    1.0,
			AsyncWorkers:      4,
			ThrottlePerSecond: 120,
			QueueTimeout:      "250ms",
		},
		Terrain: chunkServerTerrainConfig{
			Seed:        1337,
			Frequency:   0.003,
			Amplitude:   512,
			Octaves:     4,
			Persistence: 0.45,
			Lacunarity:  2.0,
		},
		Economy: chunkServerEconomyConfig{
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
		Entities: chunkServerEntitiesConfig{
			MaxEntitiesPerChunk: 4096,
			EntityTickRate:      "33ms",
			ProjectileTickRate:  "16ms",
			MovementWorkers:     1,
		},
		Environment: chunkServerEnvironmentConfig{
			DayLength:          "20m",
			WeatherMinDuration: "2m",
			WeatherMaxDuration: "5m",
			StormChance:        0.15,
			RainChance:         0.35,
			WindBase:           3.0,
			WindVariance:       5.0,
			Seed:               1337,
		},
	}
}

func (c *chunkServerConfig) applyClusterOverrides(cfg *config.Config, cs config.ChunkServer) {
	c.Server.ID = cs.ID
	c.Server.Description = "auto-generated for " + cs.ID
	c.Server.GlobalChunkOrigin = chunkServerChunkRef{X: cs.GlobalOrigin.ChunkX, Y: cs.GlobalOrigin.ChunkY}
	if cfg.World.ChunkWidth > 0 {
		c.Chunk.Width = cfg.World.ChunkWidth
	}
	if cfg.World.ChunkDepth > 0 {
		c.Chunk.Depth = cfg.World.ChunkDepth
	}
	if cfg.World.ChunkHeight > 0 {
		c.Chunk.Height = cfg.World.ChunkHeight
	}
	switch {
	case cs.ChunkSpan.ChunksX > 0:
		c.Chunk.ChunksPerAxis = cs.ChunkSpan.ChunksX
	case cs.ChunkSpan.ChunksY > 0:
		c.Chunk.ChunksPerAxis = cs.ChunkSpan.ChunksY
	}
	if cs.ListenAddress != "" {
		c.Network.ListenUDP = cs.ListenAddress
	}
}

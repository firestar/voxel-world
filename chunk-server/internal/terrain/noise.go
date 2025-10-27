package terrain

import (
	"context"
	"math"

	"chunkserver/internal/config"
	"chunkserver/internal/world"
)

// NoiseGenerator creates repeatable terrain using hashed value noise.
type NoiseGenerator struct {
	cfg            config.TerrainConfig
	economy        config.EconomyConfig
	seed           int64
	surface        int
	undergroundCap int
}

func NewNoiseGenerator(cfg config.TerrainConfig, economy config.EconomyConfig) *NoiseGenerator {
	return &NoiseGenerator{
		cfg:            cfg,
		economy:        economy,
		seed:           cfg.Seed,
		surface:        1024,
		undergroundCap: 896,
	}
}

func (g *NoiseGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	chunk := world.NewChunk(coord, bounds, dim)

	for x := 0; x < dim.Width; x++ {
		for y := 0; y < dim.Depth; y++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			globalX := bounds.Min.X + x
			globalY := bounds.Min.Y + y
			noise := g.fractalNoise(float64(globalX), float64(globalY))

			surfaceHeight := g.computeSurfaceHeight(noise)
			surfaceHeight = clampInt(surfaceHeight, bounds.Min.Z, bounds.Max.Z)

			g.populateColumn(chunk, bounds, x, y, surfaceHeight, noise)
			g.seedMinerals(chunk, x, y, globalX, globalY, bounds, dim, surfaceHeight)
		}
	}

	return chunk, nil
}

func (g *NoiseGenerator) populateColumn(chunk *world.Chunk, bounds world.Bounds, localX, localY int, surfaceHeight int, noise float64) {
	dim := chunk.Dimensions()
	maxLocalZ := surfaceHeight - bounds.Min.Z
	if maxLocalZ >= dim.Height {
		maxLocalZ = dim.Height - 1
	}
	if maxLocalZ < 0 {
		return
	}

	globalX := bounds.Min.X + localX
	globalY := bounds.Min.Y + localY

	column := make([]world.Block, maxLocalZ+1)
	for localZ := 0; localZ <= maxLocalZ; localZ++ {
		globalZ := bounds.Min.Z + localZ
		column[localZ] = g.composeTerrainBlock(globalX, globalY, globalZ, surfaceHeight, noise)
	}
	chunk.SetColumnBlocks(localX, localY, column)
}

func (g *NoiseGenerator) composeTerrainBlock(globalX, globalY, globalZ int, surfaceHeight int, noise float64) world.Block {
	depth := surfaceHeight - globalZ
	if depth < 0 {
		depth = 0
	}

	block := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       160,
		MaxHitPoints:    160,
		ConnectingForce: 120,
		Weight:          12,
		Metadata:        make(map[string]any),
	}

	switch {
	case depth <= 2:
		block.HitPoints = 90
		block.MaxHitPoints = 90
		block.ConnectingForce = 70
		block.Weight = 6
		block.Metadata["layer"] = "topsoil"
		if depth == 0 {
			world.ApplyAppearance(&block, world.MaterialGrass)
		} else {
			world.ApplyAppearance(&block, world.MaterialDirt)
		}
	case depth <= 12:
		block.HitPoints = 130
		block.MaxHitPoints = 130
		block.ConnectingForce = 95
		block.Weight = 9
		block.Metadata["layer"] = "subsoil"
		world.ApplyAppearance(&block, world.MaterialDirt)
	case depth <= 64:
		block.HitPoints = 190
		block.MaxHitPoints = 190
		block.ConnectingForce = 150
		block.Weight = 14
		block.Metadata["layer"] = "stone"
	default:
		block.HitPoints = 240
		block.MaxHitPoints = 240
		block.ConnectingForce = 210
		block.Weight = 18
		block.Metadata["layer"] = "deepstone"
	}

	if globalZ < g.undergroundCap/4 {
		block.ConnectingForce += 40
		block.Weight += 2
	}

	g.applyUnstableVariation(&block, globalX, globalY, globalZ, depth, noise)
	return block
}

func (g *NoiseGenerator) applyUnstableVariation(block *world.Block, globalX, globalY, globalZ, depth int, noise float64) {
	if depth < 6 {
		return
	}

	hashVal := hash3(globalX+depth, globalY-depth, int(g.seed)+globalZ)
	probability := float64(hashVal&0xFFFF) / 0xFFFF
	noiseBias := (noise + 1) * 0.5
	threshold := 0.05 + 0.15*noiseBias

	if probability > threshold {
		return
	}

	block.Type = world.BlockUnstable
	block.ConnectingForce *= 0.45
	block.HitPoints *= 0.8
	block.MaxHitPoints = block.HitPoints
	block.Weight *= 0.92
	block.Metadata["unstable"] = true
	block.Metadata["stabilityPenalty"] = probability
}

func (g *NoiseGenerator) computeSurfaceHeight(noise float64) int {
	return int(float64(g.surface) + noise*g.cfg.Amplitude)
}

func (g *NoiseGenerator) seedMinerals(chunk *world.Chunk, localX, localY, globalX, globalY int, bounds world.Bounds, dim world.Dimensions, surfaceHeight int) {
	for mineral, density := range g.economy.ResourceSpawnDensity {
		if density <= 0 {
			continue
		}
		hashVal := hash3(globalX, globalY, int(g.seed^int64(len(mineral))))
		value := float64(hashVal&0xFFFF) / 0xFFFF
		if value > density {
			continue
		}

		targetZ := int(float64(g.undergroundCap) * value)
		if targetZ > surfaceHeight {
			targetZ = surfaceHeight
		}
		targetZ = clampInt(targetZ, bounds.Min.Z, bounds.Max.Z)

		localZ := targetZ - bounds.Min.Z
		if localZ < 0 || localZ >= dim.Height {
			continue
		}

		block, ok := chunk.LocalBlock(localX, localY, localZ)
		if !ok || block.Type == world.BlockAir {
			continue
		}
		if block.ResourceYield == nil {
			block.ResourceYield = make(map[string]float64)
		}
		block.Type = world.BlockMineral
		block.ResourceYield[mineral] += 1
		if block.ConnectingForce < 130 {
			block.ConnectingForce = 130
		}
		if block.MaxHitPoints < 180 {
			block.MaxHitPoints = 180
		}
		if block.HitPoints < block.MaxHitPoints {
			block.HitPoints = block.MaxHitPoints
		}
		block.Weight += 3
		chunk.SetLocalBlock(localX, localY, localZ, block)
	}
}

func (g *NoiseGenerator) fractalNoise(x, y float64) float64 {
	frequency := g.cfg.Frequency
	amplitude := 1.0
	noiseSum := 0.0
	maxAmplitude := 0.0

	for i := 0; i < g.cfg.Octaves; i++ {
		noise := g.valueNoise(x*frequency, y*frequency)
		noiseSum += noise * amplitude
		maxAmplitude += amplitude
		amplitude *= g.cfg.Persistence
		frequency *= g.cfg.Lacunarity
	}

	if maxAmplitude == 0 {
		return 0
	}
	return noiseSum / maxAmplitude
}

func (g *NoiseGenerator) valueNoise(x, y float64) float64 {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1

	sx := smooth(x - float64(x0))
	sy := smooth(y - float64(y0))

	n0 := random2D(x0, y0, g.seed)
	n1 := random2D(x1, y0, g.seed)
	ix0 := lerp(n0, n1, sx)

	n2 := random2D(x0, y1, g.seed)
	n3 := random2D(x1, y1, g.seed)
	ix1 := lerp(n2, n3, sx)

	return lerp(ix0, ix1, sy)
}

func smooth(t float64) float64 {
	return t * t * (3 - 2*t)
}

func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

func random2D(x, y int, seed int64) float64 {
	return float64(hash3(x, y, int(seed))&0xFFFF)/0x8000 - 1.0
}

func hash3(x, y, z int) uint32 {
	h := uint32(x*374761393 + y*668265263 + z*2147483647)
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

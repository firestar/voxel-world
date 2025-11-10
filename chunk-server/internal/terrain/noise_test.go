package terrain

import (
	"bytes"
	"context"
	"log"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"chunkserver/internal/config"
	"chunkserver/internal/world"
)

func TestNoiseGeneratorGenerateLogsProgress(t *testing.T) {
	var buf bytes.Buffer
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	originalWriter := log.Writer()
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(originalWriter)
		log.SetPrefix(originalPrefix)
		log.SetFlags(originalFlags)
	}()

	gen := NewNoiseGenerator(config.TerrainConfig{
		Seed:        1,
		Frequency:   0.5,
		Amplitude:   1,
		Octaves:     1,
		Persistence: 0.5,
		Lacunarity:  2,
	}, config.EconomyConfig{ResourceSpawnDensity: map[string]float64{}})

	dim := world.Dimensions{Width: 2, Depth: 2, Height: 4}
	bounds := world.Bounds{
		Min: world.BlockCoord{X: 0, Y: 0, Z: 0},
		Max: world.BlockCoord{X: dim.Width - 1, Y: dim.Depth - 1, Z: dim.Height - 1},
	}

	chunk, err := gen.Generate(context.Background(), world.ChunkCoord{X: 0, Y: 0}, bounds, dim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunk == nil {
		t.Fatal("expected chunk to be generated")
	}

	logs := buf.String()
	expected := []string{"0%", "25%", "50%", "75%", "100%"}
	for _, marker := range expected {
		if !strings.Contains(logs, marker) {
			t.Fatalf("expected logs to contain progress %s, got: %s", marker, logs)
		}
	}
}

func TestNoiseGeneratorWorkerCountRespectsConfig(t *testing.T) {
	gen := NewNoiseGenerator(config.TerrainConfig{Workers: 8}, config.EconomyConfig{})
	if got := gen.workerCount(32); got != 8 {
		t.Fatalf("expected worker count to honor configuration, got %d", got)
	}
}

func TestNoiseGeneratorWorkerCountLimitsToTotalColumns(t *testing.T) {
	gen := NewNoiseGenerator(config.TerrainConfig{Workers: 16}, config.EconomyConfig{})
	if got := gen.workerCount(4); got != 4 {
		t.Fatalf("expected worker count to be limited by total columns, got %d", got)
	}

	autoGen := NewNoiseGenerator(config.TerrainConfig{}, config.EconomyConfig{})
	if got := autoGen.workerCount(1); got != 1 {
		t.Fatalf("expected automatic worker count to be at least one, got %d", got)
	}
}

func TestNoiseGeneratorMineralVeinsSpreadAcrossAxes(t *testing.T) {
	gen := NewNoiseGenerator(config.TerrainConfig{
		Seed:        99,
		Frequency:   0.01,
		Amplitude:   64,
		Octaves:     2,
		Persistence: 0.5,
		Lacunarity:  2,
	}, config.EconomyConfig{ResourceSpawnDensity: map[string]float64{"uranium": 1.0}})

	dim := world.Dimensions{Width: 6, Depth: 6, Height: 16}
	bounds := world.Bounds{
		Min: world.BlockCoord{X: 0, Y: 0, Z: 0},
		Max: world.BlockCoord{X: dim.Width - 1, Y: dim.Depth - 1, Z: dim.Height - 1},
	}

	chunk, err := gen.Generate(context.Background(), world.ChunkCoord{X: 2, Y: 3}, bounds, dim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var minerals []world.BlockCoord
	chunk.ForEachBlock(func(coord world.BlockCoord, block world.Block) bool {
		if block.Type == world.BlockMineral {
			if yield := block.ResourceYield["uranium"]; yield > 0 {
				minerals = append(minerals, coord)
			}
		}
		return true
	})

	if len(minerals) < 3 {
		t.Fatalf("expected multiple mineral blocks, got %d", len(minerals))
	}

	var horizontal, vertical, diagonal bool
	for i := 0; i < len(minerals); i++ {
		for j := i + 1; j < len(minerals); j++ {
			a, b := minerals[i], minerals[j]
			if a.X != b.X || a.Y != b.Y {
				horizontal = true
			}
			if a.Z != b.Z {
				vertical = true
			}
			diffAxes := 0
			if a.X != b.X {
				diffAxes++
			}
			if a.Y != b.Y {
				diffAxes++
			}
			if a.Z != b.Z {
				diffAxes++
			}
			if diffAxes >= 2 {
				diagonal = true
			}
		}
	}

	if !horizontal {
		t.Fatalf("expected mineral veins to spread horizontally, minerals: %#v", minerals)
	}
	if !vertical {
		t.Fatalf("expected mineral veins to span vertically, minerals: %#v", minerals)
	}
	if !diagonal {
		t.Fatalf("expected mineral veins to include diagonal growth, minerals: %#v", minerals)
	}
}

func TestNoiseGeneratorMineralVeinsDistributeAcrossColumns(t *testing.T) {
	gen := NewNoiseGenerator(config.TerrainConfig{
		Seed:        7,
		Frequency:   0.02,
		Amplitude:   48,
		Octaves:     3,
		Persistence: 0.45,
		Lacunarity:  2.2,
	}, config.EconomyConfig{ResourceSpawnDensity: map[string]float64{"titanium": 0.95}})

	dim := world.Dimensions{Width: 10, Depth: 10, Height: 32}
	bounds := world.Bounds{
		Min: world.BlockCoord{X: 0, Y: 0, Z: 0},
		Max: world.BlockCoord{X: dim.Width - 1, Y: dim.Depth - 1, Z: dim.Height - 1},
	}

	chunk, err := gen.Generate(context.Background(), world.ChunkCoord{X: 11, Y: 5}, bounds, dim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type columnKey struct {
		x int
		y int
	}

	type distribution struct {
		columns map[columnKey]struct{}
		xs      map[int]struct{}
		ys      map[int]struct{}
	}

	minerals := make(map[string]*distribution)
	chunk.ForEachBlock(func(coord world.BlockCoord, block world.Block) bool {
		if block.Type != world.BlockMineral {
			return true
		}
		for mineral, yield := range block.ResourceYield {
			if yield <= 0 {
				continue
			}
			dist, ok := minerals[mineral]
			if !ok {
				dist = &distribution{
					columns: make(map[columnKey]struct{}),
					xs:      make(map[int]struct{}),
					ys:      make(map[int]struct{}),
				}
				minerals[mineral] = dist
			}
			key := columnKey{x: coord.X, y: coord.Y}
			dist.columns[key] = struct{}{}
			dist.xs[coord.X] = struct{}{}
			dist.ys[coord.Y] = struct{}{}
		}
		return true
	})

	if len(minerals) == 0 {
		t.Fatal("expected mineral placement in generated chunk")
	}

	for mineral, dist := range minerals {
		if len(dist.columns) <= 1 {
			t.Fatalf("expected %s to occupy multiple columns, got %d", mineral, len(dist.columns))
		}
		if len(dist.xs) <= 1 {
			t.Fatalf("expected %s to span multiple X positions, columns: %#v", mineral, dist.columns)
		}
		if len(dist.ys) <= 1 {
			t.Fatalf("expected %s to span multiple Y positions, columns: %#v", mineral, dist.columns)
		}
	}
}

func TestNoiseGeneratorDeterministicAcrossRandomLocations(t *testing.T) {
	cfg := config.TerrainConfig{
		Seed:        424242,
		Frequency:   0.02,
		Amplitude:   32,
		Octaves:     2,
		Persistence: 0.55,
		Lacunarity:  2.0,
	}
	economy := config.EconomyConfig{ResourceSpawnDensity: map[string]float64{}}

	genA := NewNoiseGenerator(cfg, economy)
	genB := NewNoiseGenerator(cfg, economy)

	dim := world.Dimensions{Width: 2, Depth: 2, Height: 16}
	ctx := context.Background()

	snapshot := func(chunk *world.Chunk) map[world.BlockCoord]world.Block {
		blocks := make(map[world.BlockCoord]world.Block)
		chunk.ForEachBlock(func(coord world.BlockCoord, block world.Block) bool {
			blocks[coord] = block
			return true
		})
		return blocks
	}

	r := rand.New(rand.NewSource(1337))
	const locations = 1000
	for i := 0; i < locations; i++ {
		chunkCoord := world.ChunkCoord{
			X: r.Intn(2_000_001) - 1_000_000,
			Y: r.Intn(2_000_001) - 1_000_000,
		}
		bounds := world.Bounds{
			Min: world.BlockCoord{
				X: chunkCoord.X * dim.Width,
				Y: chunkCoord.Y * dim.Depth,
				Z: 0,
			},
			Max: world.BlockCoord{
				X: chunkCoord.X*dim.Width + dim.Width - 1,
				Y: chunkCoord.Y*dim.Depth + dim.Depth - 1,
				Z: dim.Height - 1,
			},
		}

		chunkA, err := genA.Generate(ctx, chunkCoord, bounds, dim)
		if err != nil {
			t.Fatalf("iteration %d: generator A error: %v", i, err)
		}
		chunkB, err := genB.Generate(ctx, chunkCoord, bounds, dim)
		if err != nil {
			t.Fatalf("iteration %d: generator B error: %v", i, err)
		}

		blocksA := snapshot(chunkA)
		blocksB := snapshot(chunkB)

		if len(blocksA) != len(blocksB) {
			t.Fatalf("iteration %d: block count mismatch for chunk %v: %d vs %d", i, chunkCoord, len(blocksA), len(blocksB))
		}
		for coord, blockA := range blocksA {
			blockB, ok := blocksB[coord]
			if !ok {
				t.Fatalf("iteration %d: chunk %v missing block at %v in second generation", i, chunkCoord, coord)
			}
			if !reflect.DeepEqual(blockA, blockB) {
				t.Fatalf("iteration %d: chunk %v block mismatch at %v", i, chunkCoord, coord)
			}
		}
	}
}

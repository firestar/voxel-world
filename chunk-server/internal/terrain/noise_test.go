package terrain

import (
	"bytes"
	"context"
	"log"
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

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

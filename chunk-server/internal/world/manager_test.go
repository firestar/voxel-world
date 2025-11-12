package world

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubPreviewGenerator struct {
	block Block
}

func (g *stubPreviewGenerator) Generate(ctx context.Context, coord ChunkCoord, bounds Bounds, dim Dimensions) (*Chunk, error) {
	chunk := NewChunk(coord, bounds, dim)
	chunk.SetLocalBlock(1, 1, 1, g.block)
	return chunk, nil
}

func TestManagerGeneratesChunkPreview(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	region := ServerRegion{
		Origin:        ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis: 1,
		ChunkDimension: Dimensions{
			Width:  4,
			Depth:  4,
			Height: 4,
		},
	}

	generator := &stubPreviewGenerator{
		block: Block{Type: BlockSolid, Color: "#ff0000"},
	}
	manager := NewManager(region, generator)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := manager.Chunk(ctx, ChunkCoord{X: 0, Y: 0}); err != nil {
		t.Fatalf("fetch chunk: %v", err)
	}

	previewPath := filepath.Join("chunk-preview", "chunk_0_0.png")
	deadline := time.Now().Add(time.Second)

	var img image.Image
	for {
		if time.Now().After(deadline) {
			t.Fatalf("preview not generated at %s", previewPath)
		}

		data, err := os.ReadFile(previewPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("read preview: %v", err)
		}
		if len(data) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		decoded, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		img = decoded
		break
	}

	bounds := extractBounds(img)
	if bounds.width*bounds.height <= 100 {
		t.Fatalf("preview dimensions too small: %dx%d", bounds.width, bounds.height)
	}

	if !bounds.hasNonBlackPixel {
		t.Fatalf("preview appears to be entirely black")
	}
}

type imageBounds struct {
	width            int
	height           int
	hasNonBlackPixel bool
}

func extractBounds(img image.Image) imageBounds {
	bounds := img.Bounds()
	result := imageBounds{
		width:  bounds.Dx(),
		height: bounds.Dy(),
	}

	backgroundR := uint32(10)
	backgroundG := uint32(10)
	backgroundB := uint32(18)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r >>= 8
			g >>= 8
			b >>= 8
			if r > backgroundR || g > backgroundG || b > backgroundB {
				result.hasNonBlackPixel = true
				return result
			}
		}
	}

	return result
}

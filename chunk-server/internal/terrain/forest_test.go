package terrain

import (
	"testing"

	"chunkserver/internal/config"
	"chunkserver/internal/world"
)

func TestBuildRootsStayBelowSurfaceOnSlopes(t *testing.T) {
	gen := NewNoiseGenerator(config.TerrainConfig{}, config.EconomyConfig{})
	if len(gen.treeVariants) == 0 {
		t.Fatalf("expected tree variants to be initialized")
	}

	variant := &gen.treeVariants[0]

	dim := world.Dimensions{Width: 64, Depth: 64, Height: 80}
	bounds := world.Bounds{
		Min: world.BlockCoord{X: 0, Y: 0, Z: 0},
		Max: world.BlockCoord{X: dim.Width - 1, Y: dim.Depth - 1, Z: dim.Height - 1},
	}

	chunk := world.NewChunk(world.ChunkCoord{X: 0, Y: 0}, bounds, dim)
	buffer := newChunkWriteBuffer(chunk, dim, 1<<20)

	centerX, centerY := 20, 20
	baseSurface := 30

	type coord struct{ x, y int }
	surfaces := map[coord]int{}

	setColumn := func(x, y, surface int) {
		if surface < 0 {
			surface = 0
		}
		column := make([]world.Block, surface+1)
		for i := 0; i <= surface; i++ {
			column[i] = world.Block{Type: world.BlockSolid}
		}
		buffer.setColumn(x, y, column)
		surfaces[coord{x: x, y: y}] = surface
	}

	setColumn(centerX, centerY, baseSurface)

	directions := []struct{ dx, dy int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}}
	for _, dir := range directions {
		for step := 1; step <= variant.rootReach; step++ {
			x := centerX + dir.dx*step
			y := centerY + dir.dy*step
			slopeSurface := baseSurface - step*3
			if slopeSurface < 1 {
				slopeSurface = 1
			}
			setColumn(x, y, slopeSurface)
		}
	}

	placement := treePlacement{
		localX:        centerX,
		localY:        centerY,
		surfaceLocalZ: baseSurface,
		variant:       variant,
	}

	baseLocalZ := baseSurface + 1
	gen.buildRoots(buffer, dim, placement, baseLocalZ)

	for pos, surface := range surfaces {
		if pos.x == centerX && pos.y == centerY {
			continue
		}
		column, ok := buffer.column(pos.x, pos.y)
		if !ok {
			t.Fatalf("expected column at %v to exist", pos)
		}
		highestRoot := -1
		for z := 0; z < len(column); z++ {
			block := column[z]
			if block.Metadata == nil {
				continue
			}
			if part, ok := block.Metadata["part"].(string); ok && part == "root" {
				if z > highestRoot {
					highestRoot = z
				}
			}
		}
		if highestRoot >= 0 && highestRoot >= surface {
			t.Fatalf("root at (%d,%d) placed at or above surface: rootZ=%d surface=%d", pos.x, pos.y, highestRoot, surface)
		}
	}
}

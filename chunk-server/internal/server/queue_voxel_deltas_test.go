package server

import (
	"context"
	"io"
	"log"
	"testing"

	"chunkserver/internal/world"
)

type stubGenerator struct{}

func (stubGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	return world.NewChunk(coord, bounds, dim), nil
}

func TestQueueVoxelDeltasFiltersInteriorBlocks(t *testing.T) {
	region := world.ServerRegion{
		Origin:        world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis: 1,
		ChunkDimension: world.Dimensions{
			Width:  4,
			Depth:  4,
			Height: 4,
		},
	}

	srv := &Server{
		world:  world.NewManager(region, stubGenerator{}),
		logger: log.New(io.Discard, "", 0),
	}

	chunk, err := srv.world.Chunk(context.Background(), world.ChunkCoord{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("load chunk: %v", err)
	}

	solid := world.Block{Type: world.BlockSolid}

	filled := []struct{ x, y, z int }{
		{1, 1, 1},
		{1, 1, 0},
		{1, 1, 2},
		{0, 1, 1},
		{2, 1, 1},
		{1, 0, 1},
		{1, 2, 1},
	}
	for _, c := range filled {
		if ok := chunk.SetLocalBlock(c.x, c.y, c.z, solid); !ok {
			t.Fatalf("set block %v failed", c)
		}
	}

	removalNeighbors := []struct{ x, y, z int }{
		{2, 2, 1},
		{2, 2, 3},
		{1, 2, 2},
		{3, 2, 2},
		{2, 1, 2},
		{2, 3, 2},
	}
	for _, c := range removalNeighbors {
		if ok := chunk.SetLocalBlock(c.x, c.y, c.z, solid); !ok {
			t.Fatalf("set removal neighbor %v failed", c)
		}
	}

	summary := world.NewDamageSummary()
	summary.AddChange(world.BlockChange{
		Coord:  world.BlockCoord{X: 1, Y: 1, Z: 1},
		Before: solid,
		After:  solid,
		Reason: world.ReasonDamage,
	})
	summary.AddChange(world.BlockChange{
		Coord:  world.BlockCoord{X: 0, Y: 0, Z: 0},
		Before: solid,
		After:  solid,
		Reason: world.ReasonDamage,
	})
	summary.AddChange(world.BlockChange{
		Coord:  world.BlockCoord{X: 2, Y: 2, Z: 2},
		Before: solid,
		After:  world.Block{Type: world.BlockAir},
		Reason: world.ReasonDestroy,
	})

	srv.queueVoxelDeltas(summary)

	chunkCoord := world.ChunkCoord{X: 0, Y: 0}
	byBlock := srv.deltaBuffer.data[chunkCoord]

	if len(byBlock) != 2 {
		t.Fatalf("expected 2 streamed blocks, got %d", len(byBlock))
	}
	if _, ok := byBlock[world.BlockCoord{X: 1, Y: 1, Z: 1}]; ok {
		t.Fatalf("unexpected interior block included in delta")
	}
	if _, ok := byBlock[world.BlockCoord{X: 0, Y: 0, Z: 0}]; !ok {
		t.Fatalf("surface block missing from delta")
	}
	if _, ok := byBlock[world.BlockCoord{X: 2, Y: 2, Z: 2}]; !ok {
		t.Fatalf("air change missing from delta")
	}
}

package world

import (
	"fmt"

	"chunkserver/internal/config"
)

// ChunkCoord identifies a chunk in global chunk space.
type ChunkCoord struct {
	X int
	Y int
}

// LocalChunkIndex represents a chunk index relative to the owning server region.
type LocalChunkIndex struct {
	X int
	Y int
}

// BlockCoord describes a block position in global block space.
type BlockCoord struct {
	X int
	Y int
	Z int
}

// Dimensions defines the size of a chunk in blocks.
type Dimensions struct {
	Width  int
	Depth  int
	Height int
}

// Bounds is an axis-aligned bounding box represented by inclusive min/max corners in block space.
type Bounds struct {
	Min BlockCoord
	Max BlockCoord
}

// ServerRegion delineates the contiguous grid of chunks owned by this chunk server.
type ServerRegion struct {
	Origin         ChunkCoord
	ChunksPerAxis  int
	ChunkDimension Dimensions
}

func NewServerRegion(cfg *config.Config) ServerRegion {
	return ServerRegion{
		Origin: ChunkCoord{
			X: cfg.Server.GlobalChunkOrigin.X,
			Y: cfg.Server.GlobalChunkOrigin.Y,
		},
		ChunksPerAxis: cfg.Chunk.ChunksPerAxis,
		ChunkDimension: Dimensions{
			Width:  cfg.Chunk.Width,
			Depth:  cfg.Chunk.Depth,
			Height: cfg.Chunk.Height,
		},
	}
}

func (r ServerRegion) ContainsGlobalChunk(coord ChunkCoord) bool {
	return coord.X >= r.Origin.X &&
		coord.Y >= r.Origin.Y &&
		coord.X < r.Origin.X+r.ChunksPerAxis &&
		coord.Y < r.Origin.Y+r.ChunksPerAxis
}

func (r ServerRegion) LocalToGlobalChunk(local LocalChunkIndex) (ChunkCoord, error) {
	if local.X < 0 || local.Y < 0 || local.X >= r.ChunksPerAxis || local.Y >= r.ChunksPerAxis {
		return ChunkCoord{}, fmt.Errorf("local chunk index %v out of range", local)
	}
	return ChunkCoord{
		X: r.Origin.X + local.X,
		Y: r.Origin.Y + local.Y,
	}, nil
}

func (r ServerRegion) GlobalToLocalChunk(global ChunkCoord) (LocalChunkIndex, error) {
	if !r.ContainsGlobalChunk(global) {
		return LocalChunkIndex{}, fmt.Errorf("global chunk %v not owned by region", global)
	}
	return LocalChunkIndex{
		X: global.X - r.Origin.X,
		Y: global.Y - r.Origin.Y,
	}, nil
}

func (r ServerRegion) ChunkBounds(global ChunkCoord) (Bounds, error) {
	if !r.ContainsGlobalChunk(global) {
		return Bounds{}, fmt.Errorf("chunk %v outside region", global)
	}

	min := BlockCoord{
		X: global.X * r.ChunkDimension.Width,
		Y: global.Y * r.ChunkDimension.Depth,
		Z: 0,
	}
	max := BlockCoord{
		X: min.X + r.ChunkDimension.Width - 1,
		Y: min.Y + r.ChunkDimension.Depth - 1,
		Z: r.ChunkDimension.Height - 1,
	}
	return Bounds{Min: min, Max: max}, nil
}

func (r ServerRegion) LocateBlock(block BlockCoord) (ChunkCoord, bool) {
	if block.Z < 0 || block.Z >= r.ChunkDimension.Height {
		return ChunkCoord{}, false
	}
	chunk := ChunkCoord{
		X: floorDiv(block.X, r.ChunkDimension.Width),
		Y: floorDiv(block.Y, r.ChunkDimension.Depth),
	}
	return chunk, r.ContainsGlobalChunk(chunk)
}

func floorDiv(value, size int) int {
	if size <= 0 {
		return 0
	}
	if value >= 0 {
		return value / size
	}
	return -((-value - 1) / size) - 1
}

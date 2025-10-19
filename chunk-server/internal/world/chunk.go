package world

import (
	"sync"
)

// BlockType enumerates known world block categories.
type BlockType string

const (
	BlockAir       BlockType = "air"
	BlockSolid     BlockType = "solid"
	BlockUnstable  BlockType = "unstable"
	BlockMineral   BlockType = "mineral"
	BlockExplosive BlockType = "explosive"
)

type Block struct {
	Type            BlockType
	HitPoints       float64
	MaxHitPoints    float64
	ConnectingForce float64
	Weight          float64
	ResourceYield   map[string]float64
	Metadata        map[string]any
}

// Chunk stores a dense block grid and metadata for physics and pathing.
type Chunk struct {
	Key       ChunkCoord
	Bounds    Bounds
	mu        sync.RWMutex
	blocks    map[int]Block
	dimension Dimensions
}

func NewChunk(key ChunkCoord, bounds Bounds, dim Dimensions) *Chunk {
	return &Chunk{
		Key:       key,
		Bounds:    bounds,
		blocks:    make(map[int]Block),
		dimension: dim,
	}
}

func (c *Chunk) index(localX, localY, localZ int) int {
	return (localZ*c.dimension.Depth*c.dimension.Width +
		localY*c.dimension.Width +
		localX)
}

func (c *Chunk) GlobalToLocal(coord BlockCoord) (int, int, int, bool) {
	if coord.X < c.Bounds.Min.X || coord.X > c.Bounds.Max.X ||
		coord.Y < c.Bounds.Min.Y || coord.Y > c.Bounds.Max.Y ||
		coord.Z < c.Bounds.Min.Z || coord.Z > c.Bounds.Max.Z {
		return 0, 0, 0, false
	}
	return coord.X - c.Bounds.Min.X,
		coord.Y - c.Bounds.Min.Y,
		coord.Z - c.Bounds.Min.Z, true
}

func (c *Chunk) LocalBlock(localX, localY, localZ int) (Block, bool) {
	if localX < 0 || localY < 0 || localZ < 0 ||
		localX >= c.dimension.Width || localY >= c.dimension.Depth || localZ >= c.dimension.Height {
		return Block{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	block, ok := c.blocks[c.index(localX, localY, localZ)]
	if !ok {
		return Block{Type: BlockAir}, true
	}
	return block, true
}

func (c *Chunk) SetLocalBlock(localX, localY, localZ int, block Block) bool {
	if localX < 0 || localY < 0 || localZ < 0 ||
		localX >= c.dimension.Width || localY >= c.dimension.Depth || localZ >= c.dimension.Height {
		return false
	}
	c.mu.Lock()
	idx := c.index(localX, localY, localZ)
	if block.Type == BlockAir {
		delete(c.blocks, idx)
	} else {
		c.blocks[idx] = block
	}
	c.mu.Unlock()
	return true
}

func (c *Chunk) ClearLocalBlock(localX, localY, localZ int) bool {
	return c.SetLocalBlock(localX, localY, localZ, Block{Type: BlockAir})
}

// ForEachBlock iterates over blocks, invoking fn with global coordinates.
func (c *Chunk) ForEachBlock(fn func(global BlockCoord, block Block) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for idx, block := range c.blocks {
		localX := idx % c.dimension.Width
		localY := (idx / c.dimension.Width) % c.dimension.Depth
		localZ := idx / (c.dimension.Width * c.dimension.Depth)

		global := BlockCoord{
			X: c.Bounds.Min.X + localX,
			Y: c.Bounds.Min.Y + localY,
			Z: c.Bounds.Min.Z + localZ,
		}
		if !fn(global, block) {
			return
		}
	}
}

func (c *Chunk) Dimensions() Dimensions {
	return c.dimension
}

func (c *Chunk) EvaluateColumnStability(localX, localY int) ([]StabilityReport, error) {
	return evaluateColumnStability(c, localX, localY)
}

func (c *Chunk) DamageLocalBlock(localX, localY, localZ int, amount float64) (Block, bool) {
	if amount <= 0 {
		return Block{}, false
	}
	if localX < 0 || localY < 0 || localZ < 0 ||
		localX >= c.dimension.Width || localY >= c.dimension.Depth || localZ >= c.dimension.Height {
		return Block{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.index(localX, localY, localZ)
	block, ok := c.blocks[idx]
	if !ok {
		return Block{}, false
	}
	block.HitPoints -= amount
	if block.HitPoints <= 0 {
		delete(c.blocks, idx)
		return Block{Type: BlockAir}, true
	}
	if block.MaxHitPoints > 0 && block.HitPoints > block.MaxHitPoints {
		block.HitPoints = block.MaxHitPoints
	}
	c.blocks[idx] = block
	return block, true
}

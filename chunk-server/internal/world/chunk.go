package world

import (
	"log"
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
	Material        string
	Color           string
	Texture         string
	HitPoints       float64
	MaxHitPoints    float64
	ConnectingForce float64
	Weight          float64
	ResourceYield   map[string]float64
	Metadata        map[string]any
	LightEmission   float64
}

// Chunk stores a dense block grid and metadata for physics and pathing.
type Chunk struct {
	Key       ChunkCoord
	Bounds    Bounds
	mu        sync.RWMutex
	store     BlockStorage
	dimension Dimensions
}

func NewChunk(key ChunkCoord, bounds Bounds, dim Dimensions) *Chunk {
	store, err := getStorageProvider().NewStorage(key, bounds, dim)
	if err != nil {
		log.Printf("chunk storage unavailable for %v: %v", key, err)
		store, _ = newMemoryStorageProvider().NewStorage(key, bounds, dim)
	}
	return &Chunk{
		Key:       key,
		Bounds:    bounds,
		store:     store,
		dimension: dim,
	}
}

func (c *Chunk) columnIndex(localX, localY int) int {
	return localY*c.dimension.Width + localX
}

func blockIsAir(block Block) bool {
	return block.Type == "" || block.Type == BlockAir
}

func trimColumn(column []Block) []Block {
	end := len(column)
	for end > 0 && blockIsAir(column[end-1]) {
		end--
	}
	return column[:end]
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
	idx := c.columnIndex(localX, localY)
	c.mu.RLock()
	store := c.store
	c.mu.RUnlock()
	if store == nil {
		return Block{}, false
	}
	column, ok, err := store.LoadColumn(idx)
	if err != nil {
		log.Printf("chunk %v load column %d: %v", c.Key, idx, err)
		return Block{}, false
	}
	if !ok || localZ >= len(column) || blockIsAir(column[localZ]) {
		return Block{Type: BlockAir}, true
	}
	return column[localZ], true
}

func (c *Chunk) SetLocalBlock(localX, localY, localZ int, block Block) bool {
	if localX < 0 || localY < 0 || localZ < 0 ||
		localX >= c.dimension.Width || localY >= c.dimension.Depth || localZ >= c.dimension.Height {
		return false
	}
	idx := c.columnIndex(localX, localY)
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	if store == nil {
		return false
	}
	column, ok, err := store.LoadColumn(idx)
	if err != nil {
		log.Printf("chunk %v load column %d: %v", c.Key, idx, err)
		return false
	}
	if !ok {
		column = make([]Block, localZ+1)
	} else if localZ >= len(column) {
		expanded := make([]Block, localZ+1)
		copy(expanded, column)
		column = expanded
	}
	if blockIsAir(block) {
		column[localZ] = Block{}
	} else {
		column[localZ] = block
	}
	column = trimColumn(column)
	if len(column) == 0 {
		err = store.Delete(idx)
	} else {
		err = store.SaveColumn(idx, column)
	}
	if err != nil {
		log.Printf("chunk %v persist column %d: %v", c.Key, idx, err)
		return false
	}
	return true
}

func (c *Chunk) ClearLocalBlock(localX, localY, localZ int) bool {
	return c.SetLocalBlock(localX, localY, localZ, Block{Type: BlockAir})
}

// ForEachBlock iterates over blocks, invoking fn with global coordinates.
func (c *Chunk) ForEachBlock(fn func(global BlockCoord, block Block) bool) {
	c.mu.RLock()
	store := c.store
	bounds := c.Bounds
	dim := c.dimension
	c.mu.RUnlock()

	if store == nil {
		return
	}

	if err := store.ForEach(func(idx int, column []Block) bool {
		localX := idx % dim.Width
		localY := idx / dim.Width
		for localZ, block := range column {
			if blockIsAir(block) {
				continue
			}
			global := BlockCoord{
				X: bounds.Min.X + localX,
				Y: bounds.Min.Y + localY,
				Z: bounds.Min.Z + localZ,
			}
			if !fn(global, block) {
				return false
			}
		}
		return true
	}); err != nil {
		log.Printf("chunk %v iterate blocks: %v", c.Key, err)
	}
}

func (c *Chunk) Dimensions() Dimensions {
	return c.dimension
}

// HasStoredBlocks reports whether the chunk already has any persisted block data.
func (c *Chunk) HasStoredBlocks() bool {
	c.mu.RLock()
	store := c.store
	c.mu.RUnlock()
	if store == nil {
		return false
	}

	hasBlocks := false
	if err := store.ForEach(func(_ int, column []Block) bool {
		for _, block := range column {
			if !blockIsAir(block) {
				hasBlocks = true
				return false
			}
		}
		return true
	}); err != nil {
		log.Printf("chunk %v check stored blocks: %v", c.Key, err)
	}
	return hasBlocks
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
	idx := c.columnIndex(localX, localY)
	if c.store == nil {
		return Block{}, false
	}
	column, ok, err := c.store.LoadColumn(idx)
	if err != nil {
		log.Printf("chunk %v load column %d: %v", c.Key, idx, err)
		return Block{}, false
	}
	if !ok || localZ >= len(column) || blockIsAir(column[localZ]) {
		return Block{}, false
	}
	block := column[localZ]
	block.HitPoints -= amount
	if block.HitPoints <= 0 {
		column[localZ] = Block{}
		column = trimColumn(column)
		var saveErr error
		if len(column) == 0 {
			saveErr = c.store.Delete(idx)
		} else {
			saveErr = c.store.SaveColumn(idx, column)
		}
		if saveErr != nil {
			log.Printf("chunk %v persist column %d: %v", c.Key, idx, saveErr)
			return Block{}, false
		}
		return Block{Type: BlockAir}, true
	}
	if block.MaxHitPoints > 0 && block.HitPoints > block.MaxHitPoints {
		block.HitPoints = block.MaxHitPoints
	}
	column[localZ] = block
	if err := c.store.SaveColumn(idx, trimColumn(column)); err != nil {
		log.Printf("chunk %v save column %d: %v", c.Key, idx, err)
		return Block{}, false
	}
	return block, true
}

// SetColumnBlocks replaces the entire vertical column at the given local coordinates.
func (c *Chunk) SetColumnBlocks(localX, localY int, blocks []Block) bool {
	if localX < 0 || localY < 0 || localX >= c.dimension.Width || localY >= c.dimension.Depth {
		return false
	}
	idx := c.columnIndex(localX, localY)
	column := make([]Block, len(blocks))
	copy(column, blocks)
	column = trimColumn(column)
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	if store == nil {
		return false
	}
	var err error
	if len(column) == 0 {
		err = store.Delete(idx)
	} else {
		err = store.SaveColumn(idx, column)
	}
	if err != nil {
		log.Printf("chunk %v persist column %d: %v", c.Key, idx, err)
		return false
	}
	return true
}

// Close releases any resources held by the chunk's underlying storage.
func (c *Chunk) Close() error {
	c.mu.Lock()
	store := c.store
	c.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.Close()
}

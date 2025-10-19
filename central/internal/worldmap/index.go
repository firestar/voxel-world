package worldmap

import (
	"fmt"
	"sync"

	"central/internal/config"
)

type ServerInfo struct {
	ID            string
	OriginChunkX  int
	OriginChunkY  int
	ChunksX       int
	ChunksY       int
	ListenAddress string
	HTTPAddress   string
}

type Index struct {
	mu      sync.RWMutex
	entries []ServerInfo
}

func NewIndex() *Index {
	return &Index{
		entries: make([]ServerInfo, 0),
	}
}

func (idx *Index) LoadFromConfig(cfg *config.Config) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.entries = idx.entries[:0]
	for _, cs := range cfg.ChunkServers {
		info := ServerInfo{
			ID:            cs.ID,
			OriginChunkX:  cs.GlobalOrigin.ChunkX,
			OriginChunkY:  cs.GlobalOrigin.ChunkY,
			ChunksX:       cs.ChunkSpan.ChunksX,
			ChunksY:       cs.ChunkSpan.ChunksY,
			ListenAddress: cs.ListenAddress,
			HTTPAddress:   cs.HttpAddress,
		}
		idx.entries = append(idx.entries, info)
	}
}

func (idx *Index) Lookup(blockX, blockY int, chunkWidth, chunkDepth int) (ServerInfo, error) {
	chunkX := blockX / chunkWidth
	chunkY := blockY / chunkDepth

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	for _, entry := range idx.entries {
		if chunkX >= entry.OriginChunkX &&
			chunkY >= entry.OriginChunkY &&
			chunkX < entry.OriginChunkX+entry.ChunksX &&
			chunkY < entry.OriginChunkY+entry.ChunksY {
			return entry, nil
		}
	}
	return ServerInfo{}, fmt.Errorf("no chunk server found for chunk (%d,%d)", chunkX, chunkY)
}

func (idx *Index) Servers() []ServerInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	out := make([]ServerInfo, len(idx.entries))
	copy(out, idx.entries)
	return out
}

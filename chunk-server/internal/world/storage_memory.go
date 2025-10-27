package world

import "sync"

type memoryStorageProvider struct{}

func newMemoryStorageProvider() StorageProvider {
	return &memoryStorageProvider{}
}

func (p *memoryStorageProvider) NewStorage(key ChunkCoord, bounds Bounds, dim Dimensions) (BlockStorage, error) {
	return &memoryBlockStorage{
		blocks: make(map[int][]Block),
	}, nil
}

type memoryBlockStorage struct {
	mu     sync.RWMutex
	blocks map[int][]Block
}

func (m *memoryBlockStorage) LoadColumn(index int) ([]Block, bool, error) {
	m.mu.RLock()
	blocks, ok := m.blocks[index]
	m.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	dup := make([]Block, len(blocks))
	copy(dup, blocks)
	return dup, true, nil
}

func (m *memoryBlockStorage) SaveColumn(index int, blocks []Block) error {
	m.mu.Lock()
	dup := make([]Block, len(blocks))
	copy(dup, blocks)
	m.blocks[index] = dup
	m.mu.Unlock()
	return nil
}

func (m *memoryBlockStorage) Delete(index int) error {
	m.mu.Lock()
	delete(m.blocks, index)
	m.mu.Unlock()
	return nil
}

func (m *memoryBlockStorage) ForEach(fn func(index int, blocks []Block) bool) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for idx, blocks := range m.blocks {
		dup := make([]Block, len(blocks))
		copy(dup, blocks)
		if !fn(idx, dup) {
			break
		}
	}
	return nil
}

func (m *memoryBlockStorage) Close() error {
	return nil
}

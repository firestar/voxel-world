package world

import "sync"

// BlockStorage provides persistent storage for chunk block data.
type BlockStorage interface {
	LoadColumn(index int) ([]Block, bool, error)
	SaveColumn(index int, blocks []Block) error
	Delete(index int) error
	ForEach(fn func(index int, blocks []Block) bool) error
	Close() error
}

// StorageProvider creates block storage instances for chunks.
type StorageProvider interface {
	NewStorage(key ChunkCoord, bounds Bounds, dim Dimensions) (BlockStorage, error)
}

var (
	storageProvider StorageProvider = newMemoryStorageProvider()
	storageMu       sync.RWMutex
)

// SetStorageProvider overrides the global storage provider used for new chunks.
func SetStorageProvider(provider StorageProvider) {
	storageMu.Lock()
	storageProvider = provider
	storageMu.Unlock()
}

func getStorageProvider() StorageProvider {
	storageMu.RLock()
	provider := storageProvider
	storageMu.RUnlock()
	return provider
}

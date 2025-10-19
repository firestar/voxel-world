package entities

import (
	"fmt"
	"sync"

	"chunkserver/internal/world"
)

type Manager struct {
	mu          sync.RWMutex
	entities    map[ID]*Entity
	byChunk     map[world.ChunkCoord]map[ID]*Entity
	serverID    string
}

func NewManager(serverID string) *Manager {
	return &Manager{
		entities: make(map[ID]*Entity),
		byChunk:  make(map[world.ChunkCoord]map[ID]*Entity),
		serverID: serverID,
	}
}

func (m *Manager) Add(entity *Entity) error {
	if entity == nil {
		return fmt.Errorf("nil entity")
	}
	if entity.ID == "" {
		return fmt.Errorf("entity missing id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entities[entity.ID]; exists {
		return fmt.Errorf("entity %s already registered", entity.ID)
	}
	entity.Chunk.ServerID = m.serverID
	entity.Dirty = true
	m.entities[entity.ID] = entity

	chunkSet := m.byChunk[entity.Chunk.Chunk]
	if chunkSet == nil {
		chunkSet = make(map[ID]*Entity)
		m.byChunk[entity.Chunk.Chunk] = chunkSet
	}
	chunkSet[entity.ID] = entity
	return nil
}

func (m *Manager) Remove(id ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.entities[id]
	if !ok {
		return
	}
	delete(m.entities, id)
	if chunkSet := m.byChunk[entity.Chunk.Chunk]; chunkSet != nil {
		delete(chunkSet, id)
		if len(chunkSet) == 0 {
			delete(m.byChunk, entity.Chunk.Chunk)
		}
	}
}

func (m *Manager) Transfer(id ID, newChunk world.ChunkCoord, serverID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.entities[id]
	if !ok {
		return
	}

	if chunkSet := m.byChunk[entity.Chunk.Chunk]; chunkSet != nil {
		delete(chunkSet, id)
		if len(chunkSet) == 0 {
			delete(m.byChunk, entity.Chunk.Chunk)
		}
	}

	entity.UpdateChunk(serverID, newChunk)

	set := m.byChunk[newChunk]
	if set == nil {
		set = make(map[ID]*Entity)
		m.byChunk[newChunk] = set
	}
	set[id] = entity
}

func (m *Manager) ByChunk(coord world.ChunkCoord) []Entity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chunkSet := m.byChunk[coord]
	if chunkSet == nil {
		return nil
	}
	result := make([]Entity, 0, len(chunkSet))
	for _, ent := range chunkSet {
		result = append(result, ent.Snapshot())
	}
	return result
}

func (m *Manager) Entity(id ID) (*Entity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ent, ok := m.entities[id]
	if !ok {
		return nil, false
	}
	return ent, true
}

func (m *Manager) MutableByChunk(coord world.ChunkCoord) []*Entity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	chunkSet := m.byChunk[coord]
	if len(chunkSet) == 0 {
		return nil
	}
	out := make([]*Entity, 0, len(chunkSet))
	for _, ent := range chunkSet {
		out = append(out, ent)
	}
	return out
}

// Apply executes fn for every entity and returns snapshots of those that became dirty or dying.
func (m *Manager) Apply(fn func(*Entity)) []Entity {
	m.mu.RLock()
	entities := make([]*Entity, 0, len(m.entities))
	for _, ent := range m.entities {
		entities = append(entities, ent)
	}
	m.mu.RUnlock()

	if len(entities) == 0 {
		return nil
	}

	dirtySnapshots := make([]Entity, 0, len(entities))
	toRemove := make([]ID, 0)
	for _, ent := range entities {
		fn(ent)
		snapshot := ent.Snapshot()
		if snapshot.Dirty || snapshot.Dying {
			dirtySnapshots = append(dirtySnapshots, snapshot)
			ent.MarkClean()
		}
		if snapshot.Dying {
			toRemove = append(toRemove, ent.ID)
		}
	}

	if len(toRemove) > 0 {
		m.mu.Lock()
		for _, id := range toRemove {
			entity, ok := m.entities[id]
			if !ok {
				continue
			}
			delete(m.entities, id)
			if chunkSet := m.byChunk[entity.Chunk.Chunk]; chunkSet != nil {
				delete(chunkSet, id)
				if len(chunkSet) == 0 {
					delete(m.byChunk, entity.Chunk.Chunk)
				}
			}
		}
		m.mu.Unlock()
	}

	return dirtySnapshots
}

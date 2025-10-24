package server

import (
	"sync"
	"time"

	"chunkserver/internal/config"
	"chunkserver/internal/world"
)

type neighborManager struct {
	region    world.ServerRegion
	mu        sync.RWMutex
	neighbors map[world.ChunkCoord]*neighborInfo
}

type neighborInfo struct {
	delta              world.ChunkCoord
	configuredEndpoint string
	contact            string
	remoteAddr         string
	serverID           string
	listen             string
	regionOrigin       world.ChunkCoord
	regionSize         int
	lastHello          time.Time
	lastHeard          time.Time
	connected          bool
	pendingNonce       uint64
}

type neighborTarget struct {
	Delta    world.ChunkCoord
	Endpoint string
}

type neighborOwnership struct {
	serverID string
	endpoint string
	origin   world.ChunkCoord
	size     int
}

func newNeighborManager(region world.ServerRegion, refs []config.NeighborRef) *neighborManager {
	m := &neighborManager{
		region:    region,
		neighbors: make(map[world.ChunkCoord]*neighborInfo),
	}
	for _, ref := range refs {
		delta := world.ChunkCoord{X: ref.ChunkDelta.X, Y: ref.ChunkDelta.Y}
		m.withNeighbor(delta, func(info *neighborInfo) {
			info.configuredEndpoint = ref.Endpoint
			info.contact = ref.Endpoint
		})
	}
	return m
}

func (m *neighborManager) discoveryTargets(now time.Time, interval time.Duration) []neighborTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var targets []neighborTarget
	for _, info := range m.neighbors {
		endpoint := info.contact
		if endpoint == "" {
			if info.configuredEndpoint != "" {
				endpoint = info.configuredEndpoint
			} else if info.remoteAddr != "" {
				endpoint = info.remoteAddr
			}
		}
		if endpoint == "" {
			continue
		}
		if !info.connected || interval <= 0 || now.Sub(info.lastHello) >= interval {
			targets = append(targets, neighborTarget{
				Delta:    info.delta,
				Endpoint: endpoint,
			})
		}
	}
	return targets
}

func (m *neighborManager) markHelloSent(delta world.ChunkCoord, endpoint string, nonce uint64, now time.Time) {
	m.withNeighbor(delta, func(info *neighborInfo) {
		if endpoint != "" {
			info.contact = endpoint
		}
		info.lastHello = now
		info.pendingNonce = nonce
	})
}

func (m *neighborManager) updateFromHello(addr string, listen string, serverID string, origin world.ChunkCoord, size int) world.ChunkCoord {
	delta := world.ChunkCoord{
		X: origin.X - m.region.Origin.X,
		Y: origin.Y - m.region.Origin.Y,
	}
	m.withNeighbor(delta, func(info *neighborInfo) {
		now := time.Now()
		info.remoteAddr = addr
		if listen != "" {
			info.contact = listen
		} else if info.contact == "" {
			info.contact = addr
		}
		info.serverID = serverID
		info.listen = listen
		info.regionOrigin = origin
		if size > 0 {
			info.regionSize = size
		} else {
			info.regionSize = m.region.ChunksPerAxis
		}
		info.connected = true
		info.lastHeard = now
		info.pendingNonce = 0
	})
	return delta
}

func (m *neighborManager) updateFromAck(addr string, listen string, serverID string, origin world.ChunkCoord, size int, nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var info *neighborInfo
	now := time.Now()
	if nonce != 0 {
		for _, candidate := range m.neighbors {
			if candidate.pendingNonce == nonce {
				info = candidate
				break
			}
		}
	}
	if info == nil {
		delta := world.ChunkCoord{
			X: origin.X - m.region.Origin.X,
			Y: origin.Y - m.region.Origin.Y,
		}
		info = m.ensureNeighborLocked(delta)
	}
	if listen != "" {
		info.contact = listen
	} else if info.contact == "" {
		info.contact = addr
	}
	info.remoteAddr = addr
	info.serverID = serverID
	info.listen = listen
	info.regionOrigin = origin
	if size > 0 {
		info.regionSize = size
	} else if info.regionSize == 0 {
		info.regionSize = m.region.ChunksPerAxis
	}
	info.connected = true
	info.lastHeard = now
	info.pendingNonce = 0
}

func (m *neighborManager) withNeighbor(delta world.ChunkCoord, fn func(*neighborInfo)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info := m.ensureNeighborLocked(delta)
	fn(info)
}

func (m *neighborManager) ensureNeighborLocked(delta world.ChunkCoord) *neighborInfo {
	info, ok := m.neighbors[delta]
	if !ok {
		info = &neighborInfo{
			delta: delta,
		}
		m.neighbors[delta] = info
	}
	return info
}

func (m *neighborManager) neighborForChunk(chunk world.ChunkCoord) (*neighborInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, info := range m.neighbors {
		if !info.connected {
			continue
		}
		size := info.regionSize
		if size == 0 {
			size = m.region.ChunksPerAxis
		}
		origin := info.regionOrigin
		if chunk.X >= origin.X && chunk.X < origin.X+size &&
			chunk.Y >= origin.Y && chunk.Y < origin.Y+size {
			return info, true
		}
	}
	return nil, false
}

func (info *neighborInfo) endpoint() string {
	if info.contact != "" {
		return info.contact
	}
	if info.listen != "" {
		return info.listen
	}
	return info.remoteAddr
}

func (m *neighborManager) ownership(chunk world.ChunkCoord) (neighborOwnership, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, info := range m.neighbors {
		if !info.connected {
			continue
		}
		size := info.regionSize
		if size == 0 {
			size = m.region.ChunksPerAxis
		}
		origin := info.regionOrigin
		if chunk.X >= origin.X && chunk.X < origin.X+size &&
			chunk.Y >= origin.Y && chunk.Y < origin.Y+size {
			return neighborOwnership{
				serverID: info.serverID,
				endpoint: info.endpoint(),
				origin:   origin,
				size:     size,
			}, true
		}
	}
	return neighborOwnership{}, false
}

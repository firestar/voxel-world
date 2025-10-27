package world

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// Generator describes terrain population for chunks.
type Generator interface {
	Generate(ctx context.Context, coord ChunkCoord, bounds Bounds, dim Dimensions) (*Chunk, error)
}

// Manager keeps the authoritative chunk state for this server.
type Manager struct {
	region    ServerRegion
	generator Generator

	mu     sync.RWMutex
	chunks map[ChunkCoord]*Chunk

	pending map[ChunkCoord]*chunkFuture

	lighting   LightingState
	lightingMu sync.RWMutex
}

func NewManager(region ServerRegion, generator Generator) *Manager {
	return &Manager{
		region:    region,
		generator: generator,
		chunks:    make(map[ChunkCoord]*Chunk),
		pending:   make(map[ChunkCoord]*chunkFuture),
		lighting:  DefaultLighting(),
	}
}

func (m *Manager) Region() ServerRegion {
	return m.region
}

type LightingState struct {
	Ambient     float64
	SunAngle    float64
	FogDensity  float64
	WeatherTint float64
}

func DefaultLighting() LightingState {
	return LightingState{
		Ambient:     1.0,
		SunAngle:    0,
		FogDensity:  0,
		WeatherTint: 0,
	}
}

func (m *Manager) SetLighting(state LightingState) {
	m.lightingMu.Lock()
	m.lighting = state
	m.lightingMu.Unlock()
}

func (m *Manager) Lighting() LightingState {
	m.lightingMu.RLock()
	defer m.lightingMu.RUnlock()
	return m.lighting
}

func (m *Manager) Chunk(ctx context.Context, coord ChunkCoord) (*Chunk, error) {
	if !m.region.ContainsGlobalChunk(coord) {
		return nil, fmt.Errorf("chunk %v outside server region", coord)
	}

	if ch, ok := m.cachedChunk(coord); ok {
		return ch, nil
	}

	future, err := m.ensureChunkFuture(ctx, coord)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-future.ready:
		if future.err != nil {
			return nil, future.err
		}
		return future.chunk, nil
	}
}

func (m *Manager) ChunkIfReady(coord ChunkCoord) (*Chunk, bool, error) {
	if !m.region.ContainsGlobalChunk(coord) {
		return nil, false, fmt.Errorf("chunk %v outside server region", coord)
	}

	if ch, ok := m.cachedChunk(coord); ok {
		return ch, true, nil
	}

	future, err := m.ensureChunkFuture(context.Background(), coord)
	if err != nil {
		return nil, false, err
	}

	select {
	case <-future.ready:
		if future.err != nil {
			return nil, false, future.err
		}
		return future.chunk, true, nil
	default:
		return nil, false, nil
	}
}

func (m *Manager) EnsureChunk(coord ChunkCoord) error {
	if !m.region.ContainsGlobalChunk(coord) {
		return fmt.Errorf("chunk %v outside server region", coord)
	}
	_, err := m.ensureChunkFuture(context.Background(), coord)
	return err
}

func (m *Manager) cachedChunk(coord ChunkCoord) (*Chunk, bool) {
	m.mu.RLock()
	ch, ok := m.chunks[coord]
	m.mu.RUnlock()
	return ch, ok
}

func (m *Manager) ensureChunkFuture(ctx context.Context, coord ChunkCoord) (*chunkFuture, error) {
	m.mu.Lock()
	if ch, ok := m.chunks[coord]; ok {
		m.mu.Unlock()
		future := readyChunkFuture(ch)
		return future, nil
	}
	if future, ok := m.pending[coord]; ok {
		m.mu.Unlock()
		return future, nil
	}
	future := newChunkFuture()
	m.pending[coord] = future
	m.mu.Unlock()

	bounds, err := m.region.ChunkBounds(coord)
	if err != nil {
		m.finishChunkFuture(coord, nil, err)
		return future, err
	}

	go m.generateChunk(contextWithoutCancel(ctx), coord, bounds, future)
	return future, nil
}

func (m *Manager) generateChunk(ctx context.Context, coord ChunkCoord, bounds Bounds, future *chunkFuture) {
	chunk, err := m.generator.Generate(ctx, coord, bounds, m.region.ChunkDimension)
	if err != nil {
		m.finishChunkFuture(coord, nil, err)
		return
	}
	m.finishChunkFuture(coord, chunk, nil)
}

func (m *Manager) finishChunkFuture(coord ChunkCoord, chunk *Chunk, genErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if chunk != nil {
		if existing, ok := m.chunks[coord]; ok {
			chunk = existing
		} else {
			m.chunks[coord] = chunk
		}
	}
	if future, ok := m.pending[coord]; ok {
		delete(m.pending, coord)
		future.complete(chunk, genErr)
	}
}

func contextWithoutCancel(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

type chunkFuture struct {
	ready chan struct{}
	chunk *Chunk
	err   error
	once  sync.Once
}

func newChunkFuture() *chunkFuture {
	return &chunkFuture{ready: make(chan struct{})}
}

func readyChunkFuture(chunk *Chunk) *chunkFuture {
	future := newChunkFuture()
	future.complete(chunk, nil)
	return future
}

func (f *chunkFuture) complete(chunk *Chunk, err error) {
	f.once.Do(func() {
		f.chunk = chunk
		f.err = err
		close(f.ready)
	})
}

func (m *Manager) ChunkForBlock(ctx context.Context, block BlockCoord) (*Chunk, error) {
	chunkCoord, ok := m.region.LocateBlock(block)
	if !ok {
		return nil, fmt.Errorf("block %v outside region bounds", block)
	}
	return m.Chunk(ctx, chunkCoord)
}

func (m *Manager) EvaluateColumnStability(ctx context.Context, coord ChunkCoord, localX, localY int) ([]StabilityReport, error) {
	chunk, err := m.Chunk(ctx, coord)
	if err != nil {
		return nil, err
	}
	return chunk.EvaluateColumnStability(localX, localY)
}

func (m *Manager) ApplyBlockDamage(ctx context.Context, coord BlockCoord, amount float64) (*DamageSummary, error) {
	summary := NewDamageSummary()
	if amount <= 0 {
		return summary, nil
	}

	chunkCoord, ok := m.region.LocateBlock(coord)
	if !ok {
		return summary, nil
	}

	chunk, err := m.Chunk(ctx, chunkCoord)
	if err != nil {
		return nil, err
	}

	localX, localY, localZ, ok := chunk.GlobalToLocal(coord)
	if !ok {
		return summary, nil
	}

	before, ok := chunk.LocalBlock(localX, localY, localZ)
	if !ok || before.Type == BlockAir {
		return summary, nil
	}
	beforeCopy := cloneBlock(before)

	after, changed := chunk.DamageLocalBlock(localX, localY, localZ, amount)
	if !changed {
		return summary, nil
	}

	reason := ReasonDamage
	if after.Type == BlockAir {
		reason = ReasonDestroy
	}
	summary.AddChange(BlockChange{
		Coord:  coord,
		Before: beforeCopy,
		After:  after,
		Reason: reason,
	})
	summary.AddChunk(chunkCoord)

	if err := m.cascadeColumns(ctx, []columnRef{{
		Chunk:  chunkCoord,
		LocalX: localX,
		LocalY: localY,
	}}, summary); err != nil {
		return nil, err
	}

	return summary, nil
}

func (m *Manager) ApplyExplosion(ctx context.Context, center BlockCoord, radius float64, maxDamage float64) (*DamageSummary, error) {
	summary := NewDamageSummary()
	if radius <= 0 || maxDamage <= 0 {
		return summary, nil
	}

	radiusCeil := int(math.Ceil(radius))
	minX := center.X - radiusCeil
	maxX := center.X + radiusCeil
	minY := center.Y - radiusCeil
	maxY := center.Y + radiusCeil
	minZ := center.Z - radiusCeil
	maxZ := center.Z + radiusCeil

	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			for z := minZ; z <= maxZ; z++ {
				blockCoord := BlockCoord{X: x, Y: y, Z: z}
				if blockCoord.Z < 0 {
					continue
				}
				// Skip blocks outside region early.
				if !m.region.ContainsGlobalChunk(ChunkCoord{
					X: floorDiv(blockCoord.X, m.region.ChunkDimension.Width),
					Y: floorDiv(blockCoord.Y, m.region.ChunkDimension.Depth),
				}) {
					continue
				}

				dx := float64(x - center.X)
				dy := float64(y - center.Y)
				dz := float64(z - center.Z)
				distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if distance > radius {
					continue
				}
				damage := maxDamage * (1 - distance/radius)
				if damage <= 0 {
					continue
				}
				partial, err := m.ApplyBlockDamage(ctx, blockCoord, damage)
				if err != nil {
					return nil, err
				}
				summary.Merge(partial)
			}
		}
	}

	return summary, nil
}

type columnRef struct {
	Chunk  ChunkCoord
	LocalX int
	LocalY int
}

var neighborOffsets = [...]struct{ dx, dy int }{
	{1, 0},
	{-1, 0},
	{0, 1},
	{0, -1},
}

func (m *Manager) cascadeColumns(ctx context.Context, starts []columnRef, summary *DamageSummary) error {
	if len(starts) == 0 {
		return nil
	}
	visited := make(map[columnRef]struct{})
	queue := append([]columnRef(nil), starts...)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}

		chunk, err := m.Chunk(ctx, current.Chunk)
		if err != nil {
			return err
		}

		if current.LocalX < 0 || current.LocalY < 0 ||
			current.LocalX >= m.region.ChunkDimension.Width ||
			current.LocalY >= m.region.ChunkDimension.Depth {
			continue
		}

		reports, err := chunk.EvaluateColumnStability(current.LocalX, current.LocalY)
		if err != nil {
			return err
		}

		collapsed := make([]BlockCoord, 0)
		for _, report := range reports {
			if !report.Collapsed {
				continue
			}
			chunk.ClearLocalBlock(current.LocalX, current.LocalY, report.LocalZ)
			summary.AddChange(BlockChange{
				Coord:  report.Global,
				Before: cloneBlock(report.Block),
				After:  Block{Type: BlockAir},
				Reason: ReasonCollapse,
			})
			summary.AddChunk(current.Chunk)
			collapsed = append(collapsed, report.Global)
		}

		if len(collapsed) == 0 {
			continue
		}

		for _, global := range collapsed {
			for _, offset := range neighborOffsets {
				neighbor := BlockCoord{
					X: global.X + offset.dx,
					Y: global.Y + offset.dy,
					Z: global.Z,
				}
				chunkCoord, ok := m.region.LocateBlock(neighbor)
				if !ok {
					continue
				}
				nChunk, err := m.Chunk(ctx, chunkCoord)
				if err != nil {
					return err
				}
				localX := neighbor.X - nChunk.Bounds.Min.X
				localY := neighbor.Y - nChunk.Bounds.Min.Y
				next := columnRef{
					Chunk:  chunkCoord,
					LocalX: localX,
					LocalY: localY,
				}
				if _, seen := visited[next]; !seen {
					queue = append(queue, next)
				}
			}
		}
	}

	return nil
}

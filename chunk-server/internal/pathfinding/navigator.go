package pathfinding

import (
	"container/heap"
	"context"
	"strings"
	"time"

	"chunkserver/internal/world"
)

type Mode int

const (
	ModeGround Mode = iota
	ModeFlying
	ModeUnderground
)

// UnitProfile constrains how a unit may traverse block space.
type UnitProfile struct {
	Mode      Mode
	Clearance int
	MaxClimb  int
	MaxDrop   int
	CanDig    bool
}

// BlockNavigator performs A* search over individual world blocks.
type BlockNavigator struct {
	region world.ServerRegion
	world  *world.Manager
}

func NewBlockNavigator(region world.ServerRegion, world *world.Manager) *BlockNavigator {
	return &BlockNavigator{region: region, world: world}
}

// DefaultProfile returns traversal defaults for the given unit mode.
func DefaultProfile(mode Mode) UnitProfile {
	switch mode {
	case ModeFlying:
		return UnitProfile{Mode: ModeFlying, Clearance: 2, MaxClimb: 6, MaxDrop: 6, CanDig: false}
	case ModeUnderground:
		return UnitProfile{Mode: ModeUnderground, Clearance: 1, MaxClimb: 2, MaxDrop: 6, CanDig: true}
	case ModeGround:
		fallthrough
	default:
		return UnitProfile{Mode: ModeGround, Clearance: 2, MaxClimb: 1, MaxDrop: 2, CanDig: false}
	}
}

// ModeFromString parses a textual traversal mode label.
func ModeFromString(value string) Mode {
	switch strings.ToLower(value) {
	case "flying":
		return ModeFlying
	case "underground", "digging":
		return ModeUnderground
	default:
		return ModeGround
	}
}

// FindRoute locates a block-level path subject to unit traversal constraints.
func (n *BlockNavigator) FindRoute(ctx context.Context, start, goal world.BlockCoord, profile UnitProfile) []world.BlockCoord {
	profiler := profilerFromContext(ctx)
	if start == goal {
		return []world.BlockCoord{start}
	}
	if n.world == nil {
		return nil
	}
	if _, ok := n.region.LocateBlock(start); !ok {
		return nil
	}
	if _, ok := n.region.LocateBlock(goal); !ok {
		return nil
	}

	chunkCache := make(map[world.ChunkCoord]*world.Chunk)
	if !n.passable(ctx, chunkCache, start, profile) {
		return nil
	}
	if !n.passable(ctx, chunkCache, goal, profile) {
		return nil
	}

	open := &blockQueue{}
	heap.Init(open)
	heap.Push(open, &blockPath{coord: start, priority: 0})

	cameFrom := map[world.BlockCoord]world.BlockCoord{}
	gScore := map[world.BlockCoord]int{start: 0}

	for open.Len() > 0 {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		current := heap.Pop(open).(*blockPath)
		if profiler != nil {
			profiler.RecordNodeExpanded()
		}
		if current.coord == goal {
			return reconstructBlocks(cameFrom, current.coord)
		}

		neighbors := n.neighbors(ctx, chunkCache, current.coord, profile)
		if profiler != nil {
			profiler.RecordNeighborGeneration(len(neighbors))
		}
		for _, neighbor := range neighbors {
			tentative := gScore[current.coord] + 1
			if score, ok := gScore[neighbor]; ok && tentative >= score {
				continue
			}
			cameFrom[neighbor] = current.coord
			gScore[neighbor] = tentative
			if profiler != nil {
				profiler.RecordHeuristicEvaluation()
			}
			priority := tentative + heuristicBlocks(neighbor, goal)
			heap.Push(open, &blockPath{coord: neighbor, priority: priority})
		}
	}

	return nil
}

func (n *BlockNavigator) neighbors(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord, profile UnitProfile) []world.BlockCoord {
	switch profile.Mode {
	case ModeFlying:
		return n.flyingNeighbors(ctx, cache, coord, profile)
	case ModeUnderground:
		return n.undergroundNeighbors(ctx, cache, coord, profile)
	default:
		return n.groundNeighbors(ctx, cache, coord, profile)
	}
}

func (n *BlockNavigator) groundNeighbors(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord, profile UnitProfile) []world.BlockCoord {
	offsets := [...]struct{ dx, dy int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	maxDelta := profile.MaxClimb
	if profile.MaxDrop > maxDelta {
		maxDelta = profile.MaxDrop
	}
	seen := make(map[world.BlockCoord]struct{})
	for _, offset := range offsets {
		targetX := coord.X + offset.dx
		targetY := coord.Y + offset.dy
		for delta := 0; delta <= maxDelta; delta++ {
			zOffsets := []int{}
			if delta == 0 {
				zOffsets = append(zOffsets, coord.Z)
			} else {
				if delta <= profile.MaxClimb {
					zOffsets = append(zOffsets, coord.Z+delta)
				}
				if delta <= profile.MaxDrop {
					zOffsets = append(zOffsets, coord.Z-delta)
				}
			}
			for _, targetZ := range zOffsets {
				candidate := world.BlockCoord{X: targetX, Y: targetY, Z: targetZ}
				if _, ok := seen[candidate]; ok {
					continue
				}
				if !n.passable(ctx, cache, candidate, profile) {
					continue
				}
				dz := targetZ - coord.Z
				if dz > profile.MaxClimb || dz < -profile.MaxDrop {
					continue
				}
				seen[candidate] = struct{}{}
			}
		}
	}
	neighbors := make([]world.BlockCoord, 0, len(seen))
	for candidate := range seen {
		neighbors = append(neighbors, candidate)
	}
	return neighbors
}

func (n *BlockNavigator) flyingNeighbors(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord, profile UnitProfile) []world.BlockCoord {
	offsets := [...]struct{ dx, dy, dz int }{
		{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1},
	}
	var neighbors []world.BlockCoord
	for _, offset := range offsets {
		candidate := world.BlockCoord{X: coord.X + offset.dx, Y: coord.Y + offset.dy, Z: coord.Z + offset.dz}
		dz := candidate.Z - coord.Z
		if dz > profile.MaxClimb || dz < -profile.MaxDrop {
			continue
		}
		if !n.passable(ctx, cache, candidate, profile) {
			continue
		}
		neighbors = append(neighbors, candidate)
	}
	return neighbors
}

func (n *BlockNavigator) undergroundNeighbors(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord, profile UnitProfile) []world.BlockCoord {
	// Underground traversal uses the same neighborhood as flying but respects digging constraints.
	return n.flyingNeighbors(ctx, cache, coord, profile)
}

func (n *BlockNavigator) passable(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord, profile UnitProfile) bool {
	dims := n.region.ChunkDimension
	if coord.Z < 0 || coord.Z >= dims.Height {
		return false
	}
	if _, ok := n.region.LocateBlock(coord); !ok {
		return false
	}

	for i := 0; i < profile.Clearance; i++ {
		test := world.BlockCoord{X: coord.X, Y: coord.Y, Z: coord.Z + i}
		if test.Z >= dims.Height {
			return false
		}
		block, ok := n.blockAt(ctx, cache, test)
		if !ok {
			return false
		}
		if block.Type != world.BlockAir {
			if profile.CanDig && block.Type != world.BlockSolid {
				continue
			}
			return false
		}
	}

	switch profile.Mode {
	case ModeGround:
		if coord.Z == 0 {
			return false
		}
		below := world.BlockCoord{X: coord.X, Y: coord.Y, Z: coord.Z - 1}
		block, ok := n.blockAt(ctx, cache, below)
		if !ok {
			return false
		}
		if block.Type == world.BlockAir {
			return false
		}
		return true
	default:
		return true
	}
}

func (n *BlockNavigator) blockAt(ctx context.Context, cache map[world.ChunkCoord]*world.Chunk, coord world.BlockCoord) (world.Block, bool) {
	chunkCoord, ok := n.region.LocateBlock(coord)
	if !ok {
		return world.Block{}, false
	}
	profiler := profilerFromContext(ctx)
	chunk, ok := cache[chunkCoord]
	if !ok {
		if profiler != nil {
			profiler.RecordCacheMiss()
		}
		start := time.Now()
		ch, err := n.world.Chunk(ctx, chunkCoord)
		if err != nil {
			return world.Block{}, false
		}
		if profiler != nil {
			profiler.RecordChunkLoad(time.Since(start))
		}
		chunk = ch
		cache[chunkCoord] = chunk
	} else if profiler != nil {
		profiler.RecordCacheHit()
	}
	localX, localY, localZ, ok := chunk.GlobalToLocal(coord)
	if !ok {
		return world.Block{}, false
	}
	block, ok := chunk.LocalBlock(localX, localY, localZ)
	if !ok {
		return world.Block{}, false
	}
	return block, true
}

func heuristicBlocks(a, b world.BlockCoord) int {
	dx := abs(a.X - b.X)
	dy := abs(a.Y - b.Y)
	dz := abs(a.Z - b.Z)
	return dx + dy + dz
}

func reconstructBlocks(cameFrom map[world.BlockCoord]world.BlockCoord, current world.BlockCoord) []world.BlockCoord {
	path := []world.BlockCoord{current}
	for {
		prev, ok := cameFrom[current]
		if !ok {
			break
		}
		path = append([]world.BlockCoord{prev}, path...)
		current = prev
	}
	return path
}

type blockPath struct {
	coord    world.BlockCoord
	priority int
	index    int
}

type blockQueue []*blockPath

func (q blockQueue) Len() int           { return len(q) }
func (q blockQueue) Less(i, j int) bool { return q[i].priority < q[j].priority }
func (q blockQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *blockQueue) Push(x any) {
	item := x.(*blockPath)
	item.index = len(*q)
	*q = append(*q, item)
}

func (q *blockQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*q = old[:n-1]
	return item
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

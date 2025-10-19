package pathfinding

import (
	"container/heap"

	"chunkserver/internal/world"
)

type Mode int

const (
	ModeGround Mode = iota
	ModeFlying
	ModeUnderground
)

type ChunkNavigator struct {
	region world.ServerRegion
}

func NewChunkNavigator(region world.ServerRegion) *ChunkNavigator {
	return &ChunkNavigator{region: region}
}

func (n *ChunkNavigator) FindRoute(start, goal world.ChunkCoord) []world.ChunkCoord {
	if start == goal {
		return []world.ChunkCoord{start}
	}
	if !n.region.ContainsGlobalChunk(start) && !n.region.ContainsGlobalChunk(goal) {
		return nil
	}

	open := &chunkQueue{}
	heap.Init(open)
	heap.Push(open, &chunkPath{coord: start, priority: 0})

	cameFrom := map[world.ChunkCoord]world.ChunkCoord{}
	gScore := map[world.ChunkCoord]int{start: 0}

	for open.Len() > 0 {
		current := heap.Pop(open).(*chunkPath)
		if current.coord == goal {
			return reconstruct(cameFrom, current.coord)
		}

		for _, neighbor := range neighbors(current.coord) {
			tentative := gScore[current.coord] + 1
			if score, ok := gScore[neighbor]; ok && tentative >= score {
				continue
			}
			cameFrom[neighbor] = current.coord
			gScore[neighbor] = tentative
			priority := tentative + heuristic(neighbor, goal)
			heap.Push(open, &chunkPath{coord: neighbor, priority: priority})
		}
	}

	return nil
}

func neighbors(coord world.ChunkCoord) []world.ChunkCoord {
	return []world.ChunkCoord{
		{X: coord.X + 1, Y: coord.Y},
		{X: coord.X - 1, Y: coord.Y},
		{X: coord.X, Y: coord.Y + 1},
		{X: coord.X, Y: coord.Y - 1},
	}
}

func heuristic(a, b world.ChunkCoord) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func reconstruct(cameFrom map[world.ChunkCoord]world.ChunkCoord, current world.ChunkCoord) []world.ChunkCoord {
	path := []world.ChunkCoord{current}
	for {
		prev, ok := cameFrom[current]
		if !ok {
			break
		}
		path = append([]world.ChunkCoord{prev}, path...)
		current = prev
	}
	return path
}

type chunkPath struct {
	coord    world.ChunkCoord
	priority int
	index    int
}

type chunkQueue []*chunkPath

func (q chunkQueue) Len() int           { return len(q) }
func (q chunkQueue) Less(i, j int) bool { return q[i].priority < q[j].priority }
func (q chunkQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *chunkQueue) Push(x any) {
	item := x.(*chunkPath)
	item.index = len(*q)
	*q = append(*q, item)
}

func (q *chunkQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*q = old[:n-1]
	return item
}

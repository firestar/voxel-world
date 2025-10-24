package ai

import (
	"fmt"
	"math"
	"time"

	"chunkserver/internal/world"
)

// NeighborOwnership summarizes which remote server owns a chunk outside this region.
type NeighborOwnership struct {
	ServerID     string
	Endpoint     string
	RegionOrigin world.ChunkCoord
	RegionSize   int
}

// NeighborLookup returns information about who owns the provided chunk.
type NeighborLookup func(world.ChunkCoord) (NeighborOwnership, bool)

// ConstructionPlan captures the state required to coordinate a multi-chunk build.
type ConstructionPlan struct {
	ID            string
	Blueprint     string
	Anchor        world.BlockCoord
	AssignedSquad string
	ChunkSpan     ChunkSpan
	Required      map[string]int
	Steps         []ConstructionStep
	Progress      float64
	LastEvaluated time.Time
}

// Clone returns a deep copy of the plan.
func (p *ConstructionPlan) Clone() ConstructionPlan {
	if p == nil {
		return ConstructionPlan{}
	}
	clone := *p
	if p.Required != nil {
		clone.Required = make(map[string]int, len(p.Required))
		for k, v := range p.Required {
			clone.Required[k] = v
		}
	}
	if p.Steps != nil {
		clone.Steps = append([]ConstructionStep(nil), p.Steps...)
	}
	return clone
}

// ChunkSpan describes the rectangular chunk footprint of a plan.
type ChunkSpan struct {
	Min world.ChunkCoord
	Max world.ChunkCoord
}

// ConstructionStep captures localized tasks for a specific chunk.
type ConstructionStep struct {
	Chunk       world.ChunkCoord
	LocalOrigin world.BlockCoord
	Description string
	RemoteOwner string
	Completed   bool
}

// UpdateCoverage ensures the plan spans the requested chunk radius around the anchor.
func (p *ConstructionPlan) UpdateCoverage(region world.ServerRegion, anchor world.BlockCoord, radius int, lookup NeighborLookup) {
	if radius < 0 {
		radius = 0
	}
	chunk, _ := region.LocateBlock(anchor)
	min := world.ChunkCoord{X: chunk.X - radius, Y: chunk.Y - radius}
	max := world.ChunkCoord{X: chunk.X + radius, Y: chunk.Y + radius}
	steps := make([]ConstructionStep, 0, (radius*2+1)*(radius*2+1))
	for x := min.X; x <= max.X; x++ {
		for y := min.Y; y <= max.Y; y++ {
			coord := world.ChunkCoord{X: x, Y: y}
			origin := world.BlockCoord{
				X: coord.X * region.ChunkDimension.Width,
				Y: coord.Y * region.ChunkDimension.Depth,
				Z: anchor.Z,
			}
			step := ConstructionStep{
				Chunk:       coord,
				LocalOrigin: origin,
				Description: fmt.Sprintf("stabilize foundation at chunk (%d,%d)", coord.X, coord.Y),
			}
			if !region.ContainsGlobalChunk(coord) {
				if lookup != nil {
					if owner, ok := lookup(coord); ok {
						step.RemoteOwner = owner.ServerID
						if owner.ServerID != "" {
							step.Description = fmt.Sprintf("coordinate build with %s", owner.ServerID)
						}
					} else {
						step.Description = "frontier coordination required"
					}
				}
			}
			steps = append(steps, step)
		}
	}
	p.Anchor = anchor
	p.ChunkSpan = ChunkSpan{Min: min, Max: max}
	p.Steps = steps
}

// UpdateProgress recalculates the overall plan completion based on local presence.
func (p *ConstructionPlan) UpdateProgress(active map[world.ChunkCoord]int) {
	if p == nil {
		return
	}
	if len(p.Steps) == 0 {
		p.Progress = 1
		return
	}
	var totalWeight float64
	var accumulated float64
	for idx := range p.Steps {
		step := &p.Steps[idx]
		weight := 1.0
		if step.RemoteOwner != "" {
			weight = 0.75
		}
		totalWeight += weight
		if step.RemoteOwner == "" {
			if active[step.Chunk] > 0 {
				step.Completed = true
				accumulated += weight
			} else {
				step.Completed = false
			}
			continue
		}
		// Remote step: count partial credit if adjacent chunks are covered.
		if active[step.Chunk] > 0 {
			step.Completed = true
			accumulated += weight
			continue
		}
		if hasAdjacentPresence(step.Chunk, active) {
			accumulated += weight * 0.5
			step.Completed = false
		}
	}
	if totalWeight == 0 {
		p.Progress = 0
	} else {
		p.Progress = math.Min(1, accumulated/totalWeight)
	}
	p.LastEvaluated = time.Now()
}

func hasAdjacentPresence(target world.ChunkCoord, active map[world.ChunkCoord]int) bool {
	for coord, count := range active {
		if count <= 0 {
			continue
		}
		dx := coord.X - target.X
		dy := coord.Y - target.Y
		if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
			return true
		}
	}
	return false
}

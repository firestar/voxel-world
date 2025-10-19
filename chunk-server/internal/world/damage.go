package world

type ChangeReason string

const (
	ReasonDamage   ChangeReason = "damage"
	ReasonDestroy  ChangeReason = "destroy"
	ReasonCollapse ChangeReason = "collapse"
)

var reasonPriority = map[ChangeReason]int{
	ReasonDamage:   1,
	ReasonDestroy:  2,
	ReasonCollapse: 3,
}

// BlockChange captures the before/after state of a block mutation.
type BlockChange struct {
	Coord  BlockCoord
	Before Block
	After  Block
	Reason ChangeReason
}

// DamageSummary accumulates block mutations resulting from damage application.
type DamageSummary struct {
	changes map[BlockCoord]BlockChange
	chunks  map[ChunkCoord]struct{}
}

func NewDamageSummary() *DamageSummary {
	return &DamageSummary{
		changes: make(map[BlockCoord]BlockChange),
		chunks:  make(map[ChunkCoord]struct{}),
	}
}

func (s *DamageSummary) AddChange(change BlockChange) {
	if s.changes == nil {
		s.changes = make(map[BlockCoord]BlockChange)
	}
	if existing, ok := s.changes[change.Coord]; ok {
		if reasonPriority[existing.Reason] > reasonPriority[change.Reason] {
			return
		}
		if reasonPriority[existing.Reason] == reasonPriority[change.Reason] {
			change.Before = existing.Before
		} else if reasonPriority[existing.Reason] < reasonPriority[change.Reason] {
			change.Before = existing.Before
		}
	}
	s.changes[change.Coord] = change
}

func (s *DamageSummary) AddChunk(coord ChunkCoord) {
	if s.chunks == nil {
		s.chunks = make(map[ChunkCoord]struct{})
	}
	s.chunks[coord] = struct{}{}
}

func (s *DamageSummary) Changes() []BlockChange {
	if len(s.changes) == 0 {
		return nil
	}
	out := make([]BlockChange, 0, len(s.changes))
	for _, change := range s.changes {
		out = append(out, change)
	}
	return out
}

func (s *DamageSummary) DirtyChunks() []ChunkCoord {
	if len(s.chunks) == 0 {
		return nil
	}
	out := make([]ChunkCoord, 0, len(s.chunks))
	for coord := range s.chunks {
		out = append(out, coord)
	}
	return out
}

func (s *DamageSummary) CollapsedBlocks() []BlockCoord {
	if len(s.changes) == 0 {
		return nil
	}
	out := make([]BlockCoord, 0)
	for coord, change := range s.changes {
		if change.Reason == ReasonCollapse {
			out = append(out, coord)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *DamageSummary) Merge(other *DamageSummary) {
	if other == nil {
		return
	}
	for _, change := range other.changes {
		s.AddChange(change)
	}
	for coord := range other.chunks {
		s.AddChunk(coord)
	}
}

func cloneBlock(block Block) Block {
	copy := block
	if block.ResourceYield != nil {
		copy.ResourceYield = make(map[string]float64, len(block.ResourceYield))
		for k, v := range block.ResourceYield {
			copy.ResourceYield[k] = v
		}
	}
	if block.Metadata != nil {
		copy.Metadata = make(map[string]any, len(block.Metadata))
		for k, v := range block.Metadata {
			copy.Metadata[k] = v
		}
	}
	return copy
}

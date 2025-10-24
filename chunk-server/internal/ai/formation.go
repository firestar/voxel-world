package ai

import (
	"math"

	"chunkserver/internal/world"
)

// FormationType describes how squad members are arranged relative to the anchor.
type FormationType string

const (
	// FormationLine arranges units side-by-side on the X axis.
	FormationLine FormationType = "line"
	// FormationColumn arranges units front-to-back on the Y axis.
	FormationColumn FormationType = "column"
	// FormationWedge arranges units into a triangular wedge facing forward.
	FormationWedge FormationType = "wedge"
	// FormationCircle arranges units around the anchor in a circle.
	FormationCircle FormationType = "circle"
)

// Formation captures the geometric rules for positioning a squad.
type Formation struct {
	Type    FormationType
	Anchor  world.BlockCoord
	Facing  float64
	Spacing float64
}

// SlotPosition returns the global block coordinate for the provided slot index.
func (f Formation) SlotPosition(index int) world.BlockCoord {
	if index < 0 {
		index = 0
	}
	spacing := f.Spacing
	if spacing <= 0 {
		spacing = 3
	}
	localX, localY := f.slotOffset(index)
	// Rotate around the anchor using facing (yaw).
	cos := math.Cos(f.Facing)
	sin := math.Sin(f.Facing)
	dx := cos*localX*spacing - sin*localY*spacing
	dy := sin*localX*spacing + cos*localY*spacing
	return world.BlockCoord{
		X: f.Anchor.X + int(math.Round(dx)),
		Y: f.Anchor.Y + int(math.Round(dy)),
		Z: f.Anchor.Z,
	}
}

func (f Formation) slotOffset(index int) (float64, float64) {
	switch f.Type {
	case FormationColumn:
		return 0, float64(index)
	case FormationWedge:
		return wedgeOffset(index)
	case FormationCircle:
		return circleOffset(index)
	case FormationLine:
		fallthrough
	default:
		return lineOffset(index)
	}
}

func lineOffset(index int) (float64, float64) {
	if index == 0 {
		return 0, 0
	}
	stride := (index + 1) / 2
	if index%2 == 1 {
		return float64(stride), 0
	}
	return -float64(stride), 0
}

func wedgeOffset(index int) (float64, float64) {
	// Arrange units in expanding rows: 1,2,3,...
	if index == 0 {
		return 0, 0
	}
	// Solve triangular number to find row.
	row := int(math.Floor((math.Sqrt(float64(8*index+1)) - 1) / 2))
	prev := row * (row + 1) / 2
	if prev > index {
		row--
		prev = row * (row + 1) / 2
	}
	col := index - prev
	width := float64(row + 1)
	center := (width - 1) / 2
	x := float64(col) - center
	y := float64(row + 1)
	return x, y
}

func circleOffset(index int) (float64, float64) {
	if index == 0 {
		return 0, 0
	}
	ring := int(math.Floor((math.Sqrt(float64(index))))) + 1
	ringStart := ring * ring
	if ringStart > index {
		ring--
		ringStart = ring * ring
	}
	if ring < 1 {
		ring = 1
	}
	stepsOnRing := int(math.Max(1, float64(6*ring)))
	step := index - ringStart
	angle := 2 * math.Pi * float64(step) / float64(stepsOnRing)
	radius := float64(ring)
	return radius * math.Cos(angle), radius * math.Sin(angle)
}

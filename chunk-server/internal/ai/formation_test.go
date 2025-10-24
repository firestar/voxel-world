package ai

import (
	"math"
	"testing"

	"chunkserver/internal/world"
)

func TestFormationSlotPositionLine(t *testing.T) {
	formation := Formation{Type: FormationLine, Anchor: world.BlockCoord{X: 10, Y: 20, Z: 5}, Facing: 0, Spacing: 3}
	cases := []struct {
		index int
		want  world.BlockCoord
	}{
		{0, world.BlockCoord{X: 10, Y: 20, Z: 5}},
		{1, world.BlockCoord{X: 13, Y: 20, Z: 5}},
		{2, world.BlockCoord{X: 7, Y: 20, Z: 5}},
		{3, world.BlockCoord{X: 16, Y: 20, Z: 5}},
	}
	for _, tc := range cases {
		got := formation.SlotPosition(tc.index)
		if got != tc.want {
			t.Fatalf("index %d: got %+v want %+v", tc.index, got, tc.want)
		}
	}
}

func TestFormationSlotPositionWedge(t *testing.T) {
	formation := Formation{Type: FormationWedge, Anchor: world.BlockCoord{X: 0, Y: 0, Z: 0}, Facing: math.Pi / 2, Spacing: 2}
	positions := []world.BlockCoord{}
	for i := 0; i < 5; i++ {
		positions = append(positions, formation.SlotPosition(i))
	}
	if positions[0] != (world.BlockCoord{X: 0, Y: 0, Z: 0}) {
		t.Fatalf("slot 0 unexpected: %+v", positions[0])
	}
	if positions[1].X != positions[2].X {
		t.Fatalf("first row should align along X: %+v vs %+v", positions[1], positions[2])
	}
	if positions[3].X != positions[4].X {
		t.Fatalf("second row should align along X: %+v vs %+v", positions[3], positions[4])
	}
	if positions[3].X >= positions[1].X {
		t.Fatalf("second row should extend further along facing direction: %+v vs %+v", positions[3], positions[1])
	}
	if positions[1].Y != -positions[2].Y {
		t.Fatalf("first row should mirror around anchor: %+v vs %+v", positions[1], positions[2])
	}
	if absInt(positions[3].Y-positions[4].Y) != absInt(positions[1].Y-positions[2].Y) {
		t.Fatalf("rows should maintain spacing: row1=%+v,%+v row2=%+v,%+v", positions[1], positions[2], positions[3], positions[4])
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

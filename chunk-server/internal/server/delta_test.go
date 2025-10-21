package server

import (
	"testing"
	"time"

	"chunkserver/internal/network"
	"chunkserver/internal/world"
)

func TestDeltaAccumulatorAddRespectsPriorities(t *testing.T) {
	accumulator := newDeltaAccumulator()
	chunk := world.ChunkCoord{X: 3, Y: 4}
	coord := world.BlockCoord{X: 5, Y: 6, Z: 7}

	originalBefore := world.Block{Type: world.BlockSolid, HitPoints: 15, MaxHitPoints: 20}
	originalAfter := world.Block{Type: world.BlockAir}

	accumulator.add(chunk, world.BlockChange{
		Coord:  coord,
		Before: originalBefore,
		After:  originalAfter,
		Reason: world.ReasonDestroy,
	})

	// A lower priority change should be ignored entirely.
	accumulator.add(chunk, world.BlockChange{
		Coord:  coord,
		Before: world.Block{Type: world.BlockSolid, HitPoints: 10, MaxHitPoints: 20},
		After:  world.Block{Type: world.BlockSolid, HitPoints: 8, MaxHitPoints: 20},
		Reason: world.ReasonDamage,
	})

	stored := accumulator.data[chunk][coord]
	if stored.Reason != world.ReasonDestroy {
		t.Fatalf("expected destroy change to be retained, got %v", stored.Reason)
	}
	if stored.Before.Type != originalBefore.Type ||
		stored.Before.HitPoints != originalBefore.HitPoints ||
		stored.Before.MaxHitPoints != originalBefore.MaxHitPoints {
		t.Fatalf("expected original before block to be preserved, got %#v", stored.Before)
	}

	// An equal priority change should retain the original "before" state.
	accumulator.add(chunk, world.BlockChange{
		Coord:  coord,
		Before: world.Block{Type: world.BlockSolid, HitPoints: 5, MaxHitPoints: 25},
		After:  world.Block{Type: world.BlockAir},
		Reason: world.ReasonDestroy,
	})

	stored = accumulator.data[chunk][coord]
	if stored.Before.Type != originalBefore.Type ||
		stored.Before.HitPoints != originalBefore.HitPoints ||
		stored.Before.MaxHitPoints != originalBefore.MaxHitPoints {
		t.Fatalf("expected equal priority change to keep original before block, got %#v", stored.Before)
	}
}

func TestDeltaAccumulatorFlushProducesNetworkDeltas(t *testing.T) {
	accumulator := newDeltaAccumulator()

	chunkA := world.ChunkCoord{X: 1, Y: 2}
	changeA := world.BlockChange{
		Coord:  world.BlockCoord{X: 9, Y: 10, Z: 11},
		After:  world.Block{Type: world.BlockMineral, HitPoints: 3, MaxHitPoints: 5},
		Reason: world.ReasonDamage,
	}
	chunkB := world.ChunkCoord{X: 3, Y: 4}
	changeB := world.BlockChange{
		Coord:  world.BlockCoord{X: 12, Y: 13, Z: 14},
		After:  world.Block{Type: world.BlockAir, HitPoints: 0, MaxHitPoints: 0},
		Reason: world.ReasonCollapse,
	}

	accumulator.add(chunkA, changeA)
	accumulator.add(chunkB, changeB)

	seq := uint64(100)
	deltas := accumulator.flush("server-123", &seq)

	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d", len(deltas))
	}

	if seq != 102 {
		t.Fatalf("expected sequence pointer advanced to 102, got %d", seq)
	}

	if len(accumulator.data) != 0 {
		t.Fatalf("expected accumulator to reset after flush, but still has %d entries", len(accumulator.data))
	}

	expectedByChunk := map[world.ChunkCoord]world.BlockChange{
		chunkA: changeA,
		chunkB: changeB,
	}

	seenSeq := make(map[uint64]bool)

	for _, delta := range deltas {
		if delta.ServerID != "server-123" {
			t.Errorf("unexpected server id %q", delta.ServerID)
		}
		if delta.Timestamp.IsZero() {
			t.Errorf("expected timestamp to be set")
		}
		seenSeq[delta.Seq] = true

		coord := world.ChunkCoord{X: delta.ChunkX, Y: delta.ChunkY}
		change, ok := expectedByChunk[coord]
		if !ok {
			t.Fatalf("unexpected chunk delta for %v", coord)
		}
		if len(delta.Blocks) != 1 {
			t.Fatalf("expected single block change for chunk %v, got %d", coord, len(delta.Blocks))
		}
		block := delta.Blocks[0]
		if block.X != change.Coord.X || block.Y != change.Coord.Y || block.Z != change.Coord.Z {
			t.Errorf("block coordinates mismatch: got (%d,%d,%d) want (%d,%d,%d)", block.X, block.Y, block.Z, change.Coord.X, change.Coord.Y, change.Coord.Z)
		}
		if block.Type != string(change.After.Type) {
			t.Errorf("block type mismatch: got %q want %q", block.Type, change.After.Type)
		}
		if block.HP != change.After.HitPoints || block.MaxHP != change.After.MaxHitPoints {
			t.Errorf("block hp mismatch: got (%f,%f) want (%f,%f)", block.HP, block.MaxHP, change.After.HitPoints, change.After.MaxHitPoints)
		}
		if block.Reason != string(change.Reason) {
			t.Errorf("reason mismatch: got %q want %q", block.Reason, change.Reason)
		}
		delete(expectedByChunk, coord)
	}

	if len(expectedByChunk) != 0 {
		t.Fatalf("missing deltas for chunks: %v", expectedByChunk)
	}

	if len(seenSeq) != 2 || !seenSeq[100] || !seenSeq[101] {
		t.Fatalf("expected sequential sequence numbers 100 and 101, got %v", seenSeq)
	}
}

func TestDeltaAccumulatorFlushEmptyReturnsNil(t *testing.T) {
	accumulator := newDeltaAccumulator()
	seq := uint64(5)
	if deltas := accumulator.flush("server-abc", &seq); deltas != nil {
		t.Fatalf("expected nil deltas for empty accumulator, got %#v", deltas)
	}
	if seq != 5 {
		t.Fatalf("expected sequence unchanged for empty flush, got %d", seq)
	}
}

// ensure the network import is used when tests are run on older Go versions that
// otherwise consider it unused due to build optimisations.
var _ network.ChunkDelta
var _ time.Time

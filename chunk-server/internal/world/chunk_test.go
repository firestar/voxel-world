package world

import "testing"

func TestChunkHasStoredBlocks(t *testing.T) {
	original := getStorageProvider()
	SetStorageProvider(newMemoryStorageProvider())
	t.Cleanup(func() {
		SetStorageProvider(original)
	})

	dim := Dimensions{Width: 2, Depth: 2, Height: 4}
	bounds := Bounds{
		Min: BlockCoord{X: 0, Y: 0, Z: 0},
		Max: BlockCoord{X: 1, Y: 1, Z: 3},
	}

	chunk := NewChunk(ChunkCoord{X: 0, Y: 0}, bounds, dim)
	if chunk.HasStoredBlocks() {
		t.Fatalf("expected no stored blocks for fresh chunk")
	}

	if ok := chunk.SetColumnBlocks(0, 0, []Block{{Type: BlockSolid}}); !ok {
		t.Fatalf("failed to set column blocks")
	}

	if !chunk.HasStoredBlocks() {
		t.Fatalf("expected chunk to report stored blocks after persistence")
	}
}

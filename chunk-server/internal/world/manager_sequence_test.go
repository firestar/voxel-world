package world

import (
	"context"
	"sync"
	"testing"
	"time"
)

type blockingGenerator struct {
	mu      sync.Mutex
	waiters map[ChunkCoord]chan struct{}
	notify  chan ChunkCoord
}

func newBlockingGenerator() *blockingGenerator {
	return &blockingGenerator{
		waiters: make(map[ChunkCoord]chan struct{}),
		notify:  make(chan ChunkCoord, 8),
	}
}

func (g *blockingGenerator) Generate(ctx context.Context, coord ChunkCoord, bounds Bounds, dim Dimensions) (*Chunk, error) {
	ch := make(chan struct{})
	g.mu.Lock()
	g.waiters[coord] = ch
	g.mu.Unlock()

	g.notify <- coord

	select {
	case <-ch:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	chunk := NewChunk(coord, bounds, dim)
	chunk.SetLocalBlock(0, 0, 0, Block{Type: BlockSolid})
	return chunk, nil
}

func (g *blockingGenerator) release(coord ChunkCoord) {
	g.mu.Lock()
	ch, ok := g.waiters[coord]
	if ok {
		delete(g.waiters, coord)
	}
	g.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (g *blockingGenerator) waitForCall(t *testing.T) ChunkCoord {
	t.Helper()
	select {
	case coord := <-g.notify:
		return coord
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for generation call")
	}
	return ChunkCoord{}
}

func (g *blockingGenerator) assertNoCallWithin(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case coord := <-g.notify:
		t.Fatalf("unexpected generation for %v before previous completed", coord)
	case <-time.After(d):
	}
}

func TestManagerGeneratesNextChunkAfterPreviousCompletes(t *testing.T) {
	region := ServerRegion{
		Origin:         ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: Dimensions{Width: 4, Depth: 4, Height: 4},
	}
	gen := newBlockingGenerator()
	manager := NewManager(region, gen)

	first := ChunkCoord{X: 0, Y: 0}
	second := ChunkCoord{X: 1, Y: 0}

	if err := manager.EnsureChunk(first); err != nil {
		t.Fatalf("EnsureChunk first: %v", err)
	}
	if got := gen.waitForCall(t); got != first {
		t.Fatalf("expected first generation for %v, got %v", first, got)
	}

	if err := manager.EnsureChunk(second); err != nil {
		t.Fatalf("EnsureChunk second: %v", err)
	}
	gen.assertNoCallWithin(t, 100*time.Millisecond)

	gen.release(first)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.Chunk(ctx, first); err != nil {
		t.Fatalf("Chunk first: %v", err)
	}

	if got := gen.waitForCall(t); got != second {
		t.Fatalf("expected second generation for %v, got %v", second, got)
	}
	gen.release(second)

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if _, err := manager.Chunk(ctx2, second); err != nil {
		t.Fatalf("Chunk second: %v", err)
	}
}

func TestManagerGeneratesPreviewBeforeNextChunk(t *testing.T) {
	region := ServerRegion{
		Origin:         ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: Dimensions{Width: 4, Depth: 4, Height: 4},
	}
	gen := newBlockingGenerator()
	manager := NewManager(region, gen)

	first := ChunkCoord{X: 0, Y: 0}
	second := ChunkCoord{X: 1, Y: 0}

	previewStarted := make(chan struct{})
	previewRelease := make(chan struct{})

	originalSave := saveChunkPreview
	var once sync.Once
	saveChunkPreview = func(chunk *Chunk, outputDir string) error {
		once.Do(func() {
			close(previewStarted)
		})
		<-previewRelease
		return nil
	}
	defer func() {
		saveChunkPreview = originalSave
	}()

	if err := manager.EnsureChunk(first); err != nil {
		t.Fatalf("EnsureChunk first: %v", err)
	}
	if got := gen.waitForCall(t); got != first {
		t.Fatalf("expected first generation for %v, got %v", first, got)
	}

	if err := manager.EnsureChunk(second); err != nil {
		t.Fatalf("EnsureChunk second: %v", err)
	}
	gen.assertNoCallWithin(t, 100*time.Millisecond)

	gen.release(first)

	select {
	case <-previewStarted:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for preview generation")
	}

	gen.assertNoCallWithin(t, 100*time.Millisecond)

	close(previewRelease)

	if got := gen.waitForCall(t); got != second {
		t.Fatalf("expected second generation for %v, got %v", second, got)
	}

	gen.release(second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.Chunk(ctx, second); err != nil {
		t.Fatalf("Chunk second: %v", err)
	}
}

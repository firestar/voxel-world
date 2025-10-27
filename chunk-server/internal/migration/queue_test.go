package migration

import (
	"testing"

	"chunkserver/internal/entities"
	"chunkserver/internal/world"
)

func sampleRequest(id string) Request {
	ent := entities.Entity{}
	ent.ID = entities.ID(id)
	ent.Attributes = map[string]float64{"marker": 1}
	ent.Chunk.Chunk = world.ChunkCoord{X: len(id)}
	return Request{
		EntityID:       ent.ID,
		EntitySnapshot: ent,
	}
}

func TestQueueDrainReleasesReferences(t *testing.T) {
	q := NewQueue()

	for i := 0; i < 4; i++ {
		q.Enqueue(sampleRequest(string(rune('a' + i))))
	}

	batch := q.Drain(0)
	if len(batch) != 4 {
		t.Fatalf("expected 4 requests in batch, got %d", len(batch))
	}
	if q.pending != nil {
		t.Fatalf("expected queue storage to be reset, got len=%d cap=%d", len(q.pending), cap(q.pending))
	}
	if len(batch[0].EntitySnapshot.Attributes) == 0 {
		t.Fatalf("expected drained batch to preserve snapshot data")
	}

	q.Enqueue(sampleRequest("first"))
	q.Enqueue(sampleRequest("second"))
	q.Enqueue(sampleRequest("third"))

	batch = q.Drain(2)
	if len(batch) != 2 {
		t.Fatalf("expected 2 requests in partial batch, got %d", len(batch))
	}
	if len(q.pending) != 1 {
		t.Fatalf("expected 1 request to remain in queue, got %d", len(q.pending))
	}
	if q.pending[0].EntityID != entities.ID("third") {
		t.Fatalf("expected remaining request to be 'third', got %s", q.pending[0].EntityID)
	}
}

package server

import (
	"log"
	"testing"
	"time"

	"chunkserver/internal/config"
	"chunkserver/internal/entities"
	"chunkserver/internal/migration"
)

func TestRetryStaleTransfers(t *testing.T) {
	now := time.Now()
	srv := &Server{
		cfg: &config.Config{
			Network: config.NetworkConfig{
				TransferRetry: config.Duration(2 * time.Second),
			},
		},
		migrationQueue:    migration.NewQueue(),
		inFlightTransfers: make(map[entities.ID]migration.Request),
		logger:            noopLogger(),
	}

	srv.inFlightTransfers[entities.ID("stale")] = migration.Request{
		EntityID:    entities.ID("stale"),
		LastAttempt: now.Add(-5 * time.Second),
		Nonce:       42,
	}
	srv.inFlightTransfers[entities.ID("recent")] = migration.Request{
		EntityID:    entities.ID("recent"),
		LastAttempt: now.Add(-time.Second),
		Nonce:       99,
	}
	srv.inFlightTransfers[entities.ID("unsent")] = migration.Request{
		EntityID: entities.ID("unsent"),
	}

	srv.retryStaleTransfers(now)

	if _, ok := srv.inFlightTransfers[entities.ID("stale")]; ok {
		t.Fatalf("expected stale transfer to be removed from in-flight set")
	}
	if _, ok := srv.inFlightTransfers[entities.ID("recent")]; !ok {
		t.Fatalf("expected recent transfer to remain in-flight")
	}
	if _, ok := srv.inFlightTransfers[entities.ID("unsent")]; !ok {
		t.Fatalf("expected unsent transfer to remain in-flight")
	}

	drained := srv.migrationQueue.Drain(10)
	if len(drained) != 1 {
		t.Fatalf("expected exactly one transfer to be re-queued, got %d", len(drained))
	}
	retried := drained[0]
	if retried.EntityID != entities.ID("stale") {
		t.Fatalf("expected stale transfer to be retried, got %s", retried.EntityID)
	}
	if retried.Nonce != 0 {
		t.Fatalf("expected nonce to reset on retry, got %d", retried.Nonce)
	}
	if !retried.LastAttempt.IsZero() {
		t.Fatalf("expected last attempt timestamp to reset, got %v", retried.LastAttempt)
	}
	if !retried.QueuedAt.Equal(now) {
		t.Fatalf("expected queued time to update to now, got %v", retried.QueuedAt)
	}
}

type noWriter struct{}

func (noWriter) Write(p []byte) (int, error) { return len(p), nil }

func noopLogger() *log.Logger {
	return log.New(noWriter{}, "", 0)
}

package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

type stubEntityTicker struct {
	mu      sync.Mutex
	deltas  []time.Duration
	workers []int
	notify  chan struct{}
}

func newStubEntityTicker() *stubEntityTicker {
	return &stubEntityTicker{notify: make(chan struct{}, 1)}
}

func (s *stubEntityTicker) tickEntities(delta time.Duration, workers int) {
	s.mu.Lock()
	s.deltas = append(s.deltas, delta)
	s.workers = append(s.workers, workers)
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *stubEntityTicker) waitForCalls(target int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		s.mu.Lock()
		count := len(s.deltas)
		s.mu.Unlock()
		if count >= target {
			return true
		}
		select {
		case <-s.notify:
		case <-deadline:
			return false
		}
	}
}

func (s *stubEntityTicker) snapshot() (deltas []time.Duration, workers []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deltas = append([]time.Duration(nil), s.deltas...)
	workers = append([]int(nil), s.workers...)
	return deltas, workers
}

func TestMovementEngineClampsDeltaAndUsesWorkers(t *testing.T) {
	stub := newStubEntityTicker()
	tick := 10 * time.Millisecond
	engine := newMovementEngine(stub, tick, 4)

	base := time.Unix(0, 0)
	engine.now = func() time.Time { return base }

	times := []time.Time{
		base.Add(tick),      // normal interval
		base.Add(tick),      // zero delta -> clamp
		base.Add(20 * tick), // oversized delta -> clamp
	}

	tickerChan := make(chan time.Time, len(times))
	for _, tm := range times {
		tickerChan <- tm
	}
	engine.newTicker = func(time.Duration) (<-chan time.Time, func()) {
		return tickerChan, func() {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)
	if !stub.waitForCalls(len(times), time.Second) {
		t.Fatalf("movement engine did not emit expected ticks")
	}
	cancel()
	engine.Wait()

	deltas, workers := stub.snapshot()
	if len(deltas) != len(times) {
		t.Fatalf("expected %d ticks, got %d", len(times), len(deltas))
	}
	expectedDelta := []time.Duration{tick, tick, tick}
	for i, delta := range deltas {
		if delta != expectedDelta[i] {
			t.Fatalf("tick %d delta = %v, want %v", i, delta, expectedDelta[i])
		}
	}
	for i, w := range workers {
		if w != 4 {
			t.Fatalf("tick %d workers = %d, want 4", i, w)
		}
	}
}

func TestMovementEngineDefaults(t *testing.T) {
	stub := newStubEntityTicker()
	engine := newMovementEngine(stub, 0, 0)

	if engine.tick != 33*time.Millisecond {
		t.Fatalf("default tick duration = %v, want 33ms", engine.tick)
	}
	if engine.workers != 1 {
		t.Fatalf("default workers = %d, want 1", engine.workers)
	}
	if engine.newTicker == nil {
		t.Fatalf("expected ticker factory to be initialized")
	}
	if engine.now == nil {
		t.Fatalf("expected time source to be initialized")
	}
}

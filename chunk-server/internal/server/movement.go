package server

import (
	"context"
	"sync"
	"time"
)

type movementEngine struct {
	server  *Server
	tick    time.Duration
	workers int
	wg      sync.WaitGroup
}

func newMovementEngine(server *Server, tick time.Duration, workers int) *movementEngine {
	if workers <= 0 {
		workers = 1
	}
	if tick <= 0 {
		tick = 33 * time.Millisecond
	}
	return &movementEngine{
		server:  server,
		tick:    tick,
		workers: workers,
	}
}

func (m *movementEngine) Start(ctx context.Context) {
	if m == nil || m.server == nil {
		return
	}
	m.wg.Add(1)
	go m.run(ctx)
}

func (m *movementEngine) run(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()

	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			delta := now.Sub(last)
			if delta <= 0 {
				delta = m.tick
			} else if delta > 10*m.tick {
				delta = m.tick
			}
			last = now
			m.server.tickEntities(delta, m.workers)
		}
	}
}

func (m *movementEngine) Wait() {
	if m == nil {
		return
	}
	m.wg.Wait()
}

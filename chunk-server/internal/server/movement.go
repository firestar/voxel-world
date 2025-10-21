package server

import (
	"context"
	"sync"
	"time"
)

type entityTicker interface {
	tickEntities(delta time.Duration, workers int)
}

type tickerFactory func(time.Duration) (<-chan time.Time, func())

type timeSource func() time.Time

type movementEngine struct {
	target    entityTicker
	tick      time.Duration
	workers   int
	wg        sync.WaitGroup
	newTicker tickerFactory
	now       timeSource
}

func defaultTickerFactory() tickerFactory {
	return func(d time.Duration) (<-chan time.Time, func()) {
		ticker := time.NewTicker(d)
		return ticker.C, ticker.Stop
	}
}

func newMovementEngine(target entityTicker, tick time.Duration, workers int) *movementEngine {
	if workers <= 0 {
		workers = 1
	}
	if tick <= 0 {
		tick = 33 * time.Millisecond
	}
	return &movementEngine{
		target:    target,
		tick:      tick,
		workers:   workers,
		newTicker: defaultTickerFactory(),
		now:       time.Now,
	}
}

func (m *movementEngine) Start(ctx context.Context) {
	if m == nil || m.target == nil {
		return
	}
	m.wg.Add(1)
	go m.run(ctx)
}

func (m *movementEngine) run(ctx context.Context) {
	defer m.wg.Done()
	if m.newTicker == nil {
		m.newTicker = defaultTickerFactory()
	}
	if m.now == nil {
		m.now = time.Now
	}

	tickerC, stop := m.newTicker(m.tick)
	defer stop()

	last := m.now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tickerC:
			delta := now.Sub(last)
			if delta <= 0 {
				delta = m.tick
			} else if delta > 10*m.tick {
				delta = m.tick
			}
			last = now
			m.target.tickEntities(delta, m.workers)
		}
	}
}

func (m *movementEngine) Wait() {
	if m == nil {
		return
	}
	m.wg.Wait()
}

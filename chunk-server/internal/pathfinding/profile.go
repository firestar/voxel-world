package pathfinding

import (
	"context"
	"sync/atomic"
	"time"
)

// NavigatorProfiler captures instrumentation hooks for block-level pathfinding.
type NavigatorProfiler interface {
	RecordCacheHit()
	RecordCacheMiss()
	RecordChunkLoad(duration time.Duration)
	RecordHeuristicEvaluation()
	RecordNodeExpanded()
	RecordNeighborGeneration(count int)
}

// NavigatorMetrics accumulates profiling counters for BlockNavigator operations.
type NavigatorMetrics struct {
	cacheHits            atomic.Int64
	cacheMisses          atomic.Int64
	chunkLoads           atomic.Int64
	chunkLoadTime        atomic.Int64
	heuristicEvaluations atomic.Int64
	nodesExpanded        atomic.Int64
	neighborGenerations  atomic.Int64
	neighborCount        atomic.Int64
}

// MetricsSnapshot captures a point-in-time copy of navigator metrics.
type MetricsSnapshot struct {
	CacheHits            int64
	CacheMisses          int64
	ChunkLoads           int64
	ChunkLoadTime        time.Duration
	HeuristicEvaluations int64
	NodesExpanded        int64
	NeighborGenerations  int64
	NeighborCount        int64
}

// Profiler returns a NavigatorProfiler implementation backed by this metric set.
func (m *NavigatorMetrics) Profiler() NavigatorProfiler {
	if m == nil {
		return nil
	}
	return (*metricsProfiler)(m)
}

// Reset zeroes all counters in the metrics set.
func (m *NavigatorMetrics) Reset() {
	if m == nil {
		return
	}
	m.cacheHits.Store(0)
	m.cacheMisses.Store(0)
	m.chunkLoads.Store(0)
	m.chunkLoadTime.Store(0)
	m.heuristicEvaluations.Store(0)
	m.nodesExpanded.Store(0)
	m.neighborGenerations.Store(0)
	m.neighborCount.Store(0)
}

// Snapshot captures the current counter values.
func (m *NavigatorMetrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		CacheHits:            m.cacheHits.Load(),
		CacheMisses:          m.cacheMisses.Load(),
		ChunkLoads:           m.chunkLoads.Load(),
		ChunkLoadTime:        time.Duration(m.chunkLoadTime.Load()),
		HeuristicEvaluations: m.heuristicEvaluations.Load(),
		NodesExpanded:        m.nodesExpanded.Load(),
		NeighborGenerations:  m.neighborGenerations.Load(),
		NeighborCount:        m.neighborCount.Load(),
	}
}

// metricsProfiler implements NavigatorProfiler by mutating the backing metrics set.
type metricsProfiler NavigatorMetrics

func (m *metricsProfiler) RecordCacheHit() {
	(*NavigatorMetrics)(m).cacheHits.Add(1)
}

func (m *metricsProfiler) RecordCacheMiss() {
	(*NavigatorMetrics)(m).cacheMisses.Add(1)
}

func (m *metricsProfiler) RecordChunkLoad(duration time.Duration) {
	metrics := (*NavigatorMetrics)(m)
	metrics.chunkLoads.Add(1)
	metrics.chunkLoadTime.Add(duration.Nanoseconds())
}

func (m *metricsProfiler) RecordHeuristicEvaluation() {
	(*NavigatorMetrics)(m).heuristicEvaluations.Add(1)
}

func (m *metricsProfiler) RecordNodeExpanded() {
	(*NavigatorMetrics)(m).nodesExpanded.Add(1)
}

func (m *metricsProfiler) RecordNeighborGeneration(count int) {
	metrics := (*NavigatorMetrics)(m)
	metrics.neighborGenerations.Add(1)
	metrics.neighborCount.Add(int64(count))
}

type profilerContextKey struct{}

// ContextWithProfiler returns a context that will report the provided profiler during
// pathfinding operations.
func ContextWithProfiler(ctx context.Context, profiler NavigatorProfiler) context.Context {
	if profiler == nil {
		return ctx
	}
	return context.WithValue(ctx, profilerContextKey{}, profiler)
}

func profilerFromContext(ctx context.Context) NavigatorProfiler {
	if ctx == nil {
		return nil
	}
	if profiler, ok := ctx.Value(profilerContextKey{}).(NavigatorProfiler); ok {
		return profiler
	}
	return nil
}

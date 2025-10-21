package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"chunkserver/internal/pathfinding"
	"chunkserver/internal/world"
)

type profileGenerator struct {
	dims world.Dimensions
}

func newProfileGenerator(dims world.Dimensions) *profileGenerator {
	return &profileGenerator{dims: dims}
}

func (g *profileGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	chunk := world.NewChunk(coord, bounds, dim)
	solid := world.Block{Type: world.BlockSolid, HitPoints: 100, MaxHitPoints: 100, ConnectingForce: 80, Weight: 10}

	for x := 0; x < dim.Width; x++ {
		for y := 0; y < dim.Depth; y++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			chunk.SetLocalBlock(x, y, 0, solid)

			seed := hashCoord(bounds.Min.X+x, bounds.Min.Y+y, coord.X^coord.Y)
			if seed%11 == 0 {
				height := int(seed%uint32(dim.Height/3+1)) + 1
				for z := 1; z <= height && z < dim.Height; z++ {
					chunk.SetLocalBlock(x, y, z, solid)
				}
			} else if seed%17 == 0 {
				ceiling := 2
				if ceiling < dim.Height {
					chunk.SetLocalBlock(x, y, ceiling, solid)
				}
			}
		}
	}

	return chunk, nil
}

type countingGenerator struct {
	base  world.Generator
	loads atomic.Int64
}

func newCountingGenerator(base world.Generator) *countingGenerator {
	return &countingGenerator{base: base}
}

func (g *countingGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	chunk, err := g.base.Generate(ctx, coord, bounds, dim)
	if err == nil {
		g.loads.Add(1)
	}
	return chunk, err
}

func (g *countingGenerator) LoadCount() int64 {
	return g.loads.Load()
}

type pathJob struct {
	start world.BlockCoord
	goal  world.BlockCoord
}

func main() {
	var (
		totalRequests = flag.Int("requests", 2000, "number of pathfinding requests to issue")
		concurrency   = flag.Int("concurrency", runtime.NumCPU(), "number of concurrent workers")
		chunksPerAxis = flag.Int("chunks", 3, "chunks per axis to include in the region")
		chunkWidth    = flag.Int("chunkWidth", 64, "chunk width in blocks")
		chunkDepth    = flag.Int("chunkDepth", 64, "chunk depth in blocks")
		chunkHeight   = flag.Int("chunkHeight", 96, "chunk height in blocks")
		modeFlag      = flag.String("mode", "ground", "traversal mode: ground, flying, underground")
		timeout       = flag.Duration("timeout", 250*time.Millisecond, "per-request timeout")
		seed          = flag.Int64("seed", 1337, "random seed for start/goal selection")
	)
	flag.Parse()

	if *totalRequests <= 0 {
		fmt.Fprintln(os.Stderr, "requests must be positive")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "concurrency must be positive")
		os.Exit(1)
	}
	if *chunksPerAxis <= 0 {
		fmt.Fprintln(os.Stderr, "chunks must be positive")
		os.Exit(1)
	}

	dims := world.Dimensions{Width: *chunkWidth, Depth: *chunkDepth, Height: *chunkHeight}
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  *chunksPerAxis,
		ChunkDimension: dims,
	}

	generator := newCountingGenerator(newProfileGenerator(dims))
	manager := world.NewManager(region, generator)
	navigator := pathfinding.NewBlockNavigator(region, manager)

	candidates, err := collectPassableCoordinates(context.Background(), manager, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect candidates: %v\n", err)
		os.Exit(1)
	}
	if len(candidates) < 2 {
		fmt.Fprintln(os.Stderr, "not enough passable coordinates to profile")
		os.Exit(1)
	}

	jobs := make(chan pathJob)
	go func() {
		defer close(jobs)
		rng := rand.New(rand.NewSource(*seed))
		for i := 0; i < *totalRequests; i++ {
			start := candidates[rng.Intn(len(candidates))]
			goal := candidates[rng.Intn(len(candidates))]
			for start == goal {
				goal = candidates[rng.Intn(len(candidates))]
			}
			jobs <- pathJob{start: start, goal: goal}
		}
	}()

	ctx := context.Background()
	profile := pathfinding.DefaultProfile(pathfinding.ModeFromString(*modeFlag))

	var (
		wg                 sync.WaitGroup
		totalNodes         int64
		totalHeuristics    int64
		totalHits          int64
		totalMisses        int64
		totalSuccessLength int64
		totalRouteDuration int64
		successes          int64
		failures           int64
		timeouts           int64
	)

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			routeCtx, cancel := context.WithTimeout(ctx, *timeout)
			startTime := time.Now()
			path, stats := navigator.FindRouteWithStats(routeCtx, job.start, job.goal, profile)
			duration := time.Since(startTime)
			cancel()

			atomic.AddInt64(&totalNodes, int64(stats.NodesExpanded))
			atomic.AddInt64(&totalHeuristics, int64(stats.HeuristicEvaluations))
			atomic.AddInt64(&totalHits, int64(stats.CacheHits))
			atomic.AddInt64(&totalMisses, int64(stats.CacheMisses))
			atomic.AddInt64(&totalRouteDuration, int64(duration))

			if routeCtx.Err() == context.DeadlineExceeded {
				atomic.AddInt64(&timeouts, 1)
				continue
			}
			if len(path) == 0 {
				atomic.AddInt64(&failures, 1)
				continue
			}
			atomic.AddInt64(&successes, 1)
			atomic.AddInt64(&totalSuccessLength, int64(len(path)-1))
		}
	}

	wg.Add(*concurrency)
	for i := 0; i < *concurrency; i++ {
		go worker()
	}

	startWall := time.Now()
	wg.Wait()
	wallDuration := time.Since(startWall)

	totalRequests64 := int64(*totalRequests)
	avgDuration := time.Duration(0)
	if totalRequests64 > 0 {
		avgDuration = time.Duration(totalRouteDuration / totalRequests64)
	}
	hitRatio := 0.0
	hits := atomic.LoadInt64(&totalHits)
	misses := atomic.LoadInt64(&totalMisses)
	if hits+misses > 0 {
		hitRatio = float64(hits) / float64(hits+misses) * 100
	}
	avgPathLength := 0.0
	succ := atomic.LoadInt64(&successes)
	if succ > 0 {
		avgPathLength = float64(atomic.LoadInt64(&totalSuccessLength)) / float64(succ)
	}

	fmt.Println("== Block Pathfinding Profile ==")
	fmt.Printf("Chunks per axis: %d\n", *chunksPerAxis)
	fmt.Printf("Chunk dimensions: %dx%dx%d\n", dims.Width, dims.Depth, dims.Height)
	fmt.Printf("Mode: %s\n", *modeFlag)
	fmt.Printf("Requests: %d\n", *totalRequests)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Successes: %d, Failures: %d, Timeouts: %d\n", succ, atomic.LoadInt64(&failures), atomic.LoadInt64(&timeouts))
	fmt.Printf("Average path length (steps): %.2f\n", avgPathLength)
	fmt.Printf("Average per-route duration: %s\n", avgDuration)
	fmt.Printf("Wall clock duration: %s\n", wallDuration)
	fmt.Printf("Average nodes expanded: %.2f\n", float64(atomic.LoadInt64(&totalNodes))/float64(totalRequests64))
	fmt.Printf("Average heuristic evaluations: %.2f\n", float64(atomic.LoadInt64(&totalHeuristics))/float64(totalRequests64))
	fmt.Printf("Cache hit ratio: %.2f%% (%d hits, %d misses)\n", hitRatio, hits, misses)
	fmt.Printf("Chunks generated: %d\n", generator.LoadCount())
}

func collectPassableCoordinates(ctx context.Context, manager *world.Manager, region world.ServerRegion) ([]world.BlockCoord, error) {
	dims := region.ChunkDimension
	coords := make([]world.BlockCoord, 0, dims.Width*dims.Depth*region.ChunksPerAxis*region.ChunksPerAxis)
	for x := 0; x < region.ChunksPerAxis; x++ {
		for y := 0; y < region.ChunksPerAxis; y++ {
			chunkCoord := world.ChunkCoord{X: region.Origin.X + x, Y: region.Origin.Y + y}
			chunk, err := manager.Chunk(ctx, chunkCoord)
			if err != nil {
				return nil, err
			}
			bounds := chunk.Bounds
			for localX := 0; localX < dims.Width; localX++ {
				for localY := 0; localY < dims.Depth; localY++ {
					surface := highestSolid(chunk, localX, localY)
					targetZ := surface + 1
					if targetZ >= dims.Height {
						continue
					}
					coord := world.BlockCoord{
						X: bounds.Min.X + localX,
						Y: bounds.Min.Y + localY,
						Z: bounds.Min.Z + targetZ,
					}
					coords = append(coords, coord)
				}
			}
		}
	}
	return coords, nil
}

func highestSolid(chunk *world.Chunk, localX, localY int) int {
	dims := chunk.Dimensions()
	lastSolid := -1
	for z := 0; z < dims.Height; z++ {
		block, ok := chunk.LocalBlock(localX, localY, z)
		if !ok {
			break
		}
		if block.Type != world.BlockAir {
			lastSolid = z
		}
	}
	if lastSolid == -1 {
		return 0
	}
	return lastSolid
}

func hashCoord(x, y, z int) uint32 {
	h := uint32(x*374761393 + y*668265263 + z*362437)
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

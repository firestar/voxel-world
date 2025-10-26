package pathfinding

import (
	"context"
	"testing"

	"chunkserver/internal/world"
)

type stubGenerator struct {
	chunks map[world.ChunkCoord]*world.Chunk
}

func newStubGenerator() *stubGenerator {
	return &stubGenerator{chunks: make(map[world.ChunkCoord]*world.Chunk)}
}

func (g *stubGenerator) setChunk(coord world.ChunkCoord, chunk *world.Chunk) {
	g.chunks[coord] = chunk
}

func (g *stubGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	if chunk, ok := g.chunks[coord]; ok {
		return chunk, nil
	}
	chunk := world.NewChunk(coord, bounds, dim)
	g.chunks[coord] = chunk
	return chunk, nil
}

func newTestNavigator(t *testing.T, dims world.Dimensions) (*BlockNavigator, *world.Chunk) {
	t.Helper()

	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  1,
		ChunkDimension: dims,
	}

	chunkCoord := world.ChunkCoord{X: 0, Y: 0}
	bounds := world.Bounds{
		Min: world.BlockCoord{X: 0, Y: 0, Z: 0},
		Max: world.BlockCoord{X: dims.Width - 1, Y: dims.Depth - 1, Z: dims.Height - 1},
	}

	chunk := world.NewChunk(chunkCoord, bounds, dims)

	generator := newStubGenerator()
	generator.setChunk(chunkCoord, chunk)

	manager := world.NewManager(region, generator)
	navigator := NewBlockNavigator(region, manager)

	return navigator, chunk
}

func newNavigatorWithRegion(t *testing.T, region world.ServerRegion) (*BlockNavigator, *world.Manager, *stubGenerator) {
	t.Helper()

	generator := newStubGenerator()
	manager := world.NewManager(region, generator)
	navigator := NewBlockNavigator(region, manager)

	return navigator, manager, generator
}

func newChunkForRegion(t *testing.T, region world.ServerRegion, coord world.ChunkCoord) *world.Chunk {
	t.Helper()

	bounds, err := region.ChunkBounds(coord)
	if err != nil {
		t.Fatalf("unable to compute bounds for chunk %v: %v", coord, err)
	}
	return world.NewChunk(coord, bounds, region.ChunkDimension)
}

func addFloor(chunk *world.Chunk, height int) {
	for x := 0; x < chunk.Dimensions().Width; x++ {
		for y := 0; y < chunk.Dimensions().Depth; y++ {
			chunk.SetLocalBlock(x, y, height, world.Block{Type: world.BlockSolid})
		}
	}
}

func surroundWithWalls(chunk *world.Chunk, height int) {
	dims := chunk.Dimensions()
	for x := 0; x < dims.Width; x++ {
		for z := 0; z <= height; z++ {
			chunk.SetLocalBlock(x, 0, z, world.Block{Type: world.BlockSolid})
			chunk.SetLocalBlock(x, dims.Depth-1, z, world.Block{Type: world.BlockSolid})
		}
	}
	for y := 0; y < dims.Depth; y++ {
		for z := 0; z <= height; z++ {
			chunk.SetLocalBlock(0, y, z, world.Block{Type: world.BlockSolid})
			chunk.SetLocalBlock(dims.Width-1, y, z, world.Block{Type: world.BlockSolid})
		}
	}
}

func TestBlockNavigatorGroundRouteAvoidsObstacles(t *testing.T) {
	dims := world.Dimensions{Width: 6, Depth: 6, Height: 6}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Place a pillar that should force a detour around (1,1,1).
	chunk.SetLocalBlock(1, 1, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(1, 1, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 4, Y: 1, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path to be found, got none")
	}
	if path[0] != start {
		t.Fatalf("path should start at %v, got %v", start, path[0])
	}
	if path[len(path)-1] != goal {
		t.Fatalf("path should end at %v, got %v", goal, path[len(path)-1])
	}

	for _, step := range path {
		if step == (world.BlockCoord{X: 1, Y: 1, Z: 1}) {
			t.Fatalf("path traversed blocked coordinate %v", step)
		}
	}

	detour := false
	for i, step := range path {
		if i == 0 || i == len(path)-1 {
			continue
		}
		if step.Y != start.Y {
			detour = true
			break
		}
	}
	if !detour {
		t.Fatalf("expected path to detour around obstacle, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteRespectsClimbLimit(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 3, Height: 6}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Create a platform two blocks higher than the start.
	chunk.SetLocalBlock(1, 1, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 1, Y: 1, Z: 3}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if path != nil {
		t.Fatalf("expected no path due to climb limit, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteRespectsDropLimit(t *testing.T) {
	dims := world.Dimensions{Width: 2, Depth: 2, Height: 6}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Support the starting position at Z=4.
	chunk.SetLocalBlock(0, 0, 3, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 4}
	goal := world.BlockCoord{X: 1, Y: 0, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if path != nil {
		t.Fatalf("expected no path due to drop limit, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteRequiresClearance(t *testing.T) {
	dims := world.Dimensions{Width: 5, Depth: 3, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	surroundWithWalls(chunk, 3)

	// Corridor at y=1 with a low ceiling in the middle that blocks default clearance of 2.
	for x := 1; x <= 3; x++ {
		chunk.ClearLocalBlock(x, 1, 1)
		chunk.ClearLocalBlock(x, 1, 2)
	}

	chunk.SetLocalBlock(2, 1, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 3, Y: 1, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if path != nil {
		t.Fatalf("expected no path due to insufficient clearance, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteWithReducedClearanceSucceeds(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Leave a low ceiling at Z=2 so only one vertical block of air is available.
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	tightProfile := DefaultProfile(ModeGround)
	tightProfile.Clearance = 1

	path := navigator.FindRoute(context.Background(), start, goal, tightProfile)
	if len(path) == 0 {
		t.Fatalf("expected path through low corridor, got none")
	}
	if path[0] != start || path[len(path)-1] != goal {
		t.Fatalf("unexpected endpoints for path %v", path)
	}
	for _, step := range path {
		if step == (world.BlockCoord{X: 1, Y: 0, Z: 1}) {
			return
		}
	}
	t.Fatalf("path did not traverse the corridor block: %v", path)
}

func TestBlockNavigatorFlyingRouteDetoursVertically(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 1, Height: 5}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Build a pillar that blocks the direct path at Z=2.
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 2}
	goal := world.BlockCoord{X: 3, Y: 0, Z: 2}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying))
	if len(path) == 0 {
		t.Fatalf("expected flying unit to find path over obstacle")
	}
	climbed := false
	for _, step := range path {
		if step.Z > start.Z {
			climbed = true
		}
	}
	if !climbed {
		t.Fatalf("expected flying path to climb above obstacle, got %v", path)
	}
	if path[0] != start || path[len(path)-1] != goal {
		t.Fatalf("unexpected endpoints for path %v", path)
	}
}

func TestBlockNavigatorUndergroundRouteCanTunnelThroughMineral(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Mineral deposit blocking the corridor.
	chunk.SetLocalBlock(1, 0, 1, world.Block{Type: world.BlockMineral})
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	groundPath := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if groundPath != nil {
		t.Fatalf("ground profile should fail through mineral deposit, got %v", groundPath)
	}

	tunnelPath := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeUnderground))
	if len(tunnelPath) == 0 {
		t.Fatalf("expected underground profile to tunnel through mineral deposit")
	}
	if tunnelPath[0] != start || tunnelPath[len(tunnelPath)-1] != goal {
		t.Fatalf("unexpected tunnel path %v", tunnelPath)
	}
}

func TestBlockNavigatorGroundRouteRejectsBlockedEndpoints(t *testing.T) {
	dims := world.Dimensions{Width: 5, Depth: 3, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 3, Y: 1, Z: 1}

	chunk.SetLocalBlock(start.X, start.Y, start.Z, world.Block{Type: world.BlockSolid})
	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected no path when start block is occupied, got %v", path)
	}

	chunk.ClearLocalBlock(start.X, start.Y, start.Z)
	chunk.SetLocalBlock(goal.X, goal.Y, goal.Z, world.Block{Type: world.BlockSolid})
	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected no path when goal block is occupied, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteStepsArePassable(t *testing.T) {
	dims := world.Dimensions{Width: 5, Depth: 3, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Column blocking the direct path that forces a detour.
	chunk.SetLocalBlock(2, 1, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(2, 1, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 4, Y: 1, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected ground route around obstacle")
	}
	cache := make(map[world.ChunkCoord]*world.Chunk)
	profile := DefaultProfile(ModeGround)
	for idx, step := range path {
		if !navigator.passable(context.Background(), cache, step, profile) {
			t.Fatalf("path step %d (%v) is not passable", idx, step)
		}
	}
}

func TestBlockNavigatorProfilerRecordsMetrics(t *testing.T) {
	dims := world.Dimensions{Width: 6, Depth: 6, Height: 6}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	// Place a pillar to force the search to evaluate multiple neighbors.
	chunk.SetLocalBlock(2, 2, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(2, 2, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 5, Y: 5, Z: 1}

	metrics := &NavigatorMetrics{}
	ctx := ContextWithProfiler(context.Background(), metrics.Profiler())

	path := navigator.FindRoute(ctx, start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path to be found with profiling enabled")
	}

	snapshot := metrics.Snapshot()
	if snapshot.CacheMisses == 0 {
		t.Fatalf("expected cache misses to be recorded, got %#v", snapshot)
	}
	if snapshot.CacheHits == 0 {
		t.Fatalf("expected cache hits to be recorded, got %#v", snapshot)
	}
	if snapshot.ChunkLoads == 0 {
		t.Fatalf("expected chunk load count to be recorded, got %#v", snapshot)
	}
	if snapshot.HeuristicEvaluations == 0 {
		t.Fatalf("expected heuristic evaluations to be recorded, got %#v", snapshot)
	}
	if snapshot.NodesExpanded == 0 {
		t.Fatalf("expected node expansions to be recorded, got %#v", snapshot)
	}
	if snapshot.NeighborGenerations == 0 || snapshot.NeighborCount == 0 {
		t.Fatalf("expected neighbor generation metrics to be recorded, got %#v", snapshot)
	}
}

func TestNavigatorMetricsReset(t *testing.T) {
	metrics := &NavigatorMetrics{}
	profiler := metrics.Profiler()

	profiler.RecordCacheHit()
	profiler.RecordCacheMiss()
	profiler.RecordChunkLoad(5)
	profiler.RecordHeuristicEvaluation()
	profiler.RecordNodeExpanded()
	profiler.RecordNeighborGeneration(3)

	snapshot := metrics.Snapshot()
	if snapshot.CacheHits == 0 || snapshot.CacheMisses == 0 || snapshot.ChunkLoads == 0 {
		t.Fatalf("expected metrics snapshot to include recorded values, got %#v", snapshot)
	}

	metrics.Reset()
	cleared := metrics.Snapshot()
	if cleared.CacheHits != 0 || cleared.CacheMisses != 0 || cleared.ChunkLoads != 0 || cleared.ChunkLoadTime != 0 ||
		cleared.HeuristicEvaluations != 0 || cleared.NodesExpanded != 0 || cleared.NeighborGenerations != 0 ||
		cleared.NeighborCount != 0 {
		t.Fatalf("expected reset metrics to be zeroed, got %#v", cleared)
	}
}

func TestBlockNavigatorGroundRouteStopsAtCanyon(t *testing.T) {
	dims := world.Dimensions{Width: 6, Depth: 3, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)

	for y := 0; y < dims.Depth; y++ {
		for z := 0; z < 3; z++ {
			chunk.ClearLocalBlock(3, y, z)
		}
	}

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 5, Y: 1, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected canyon to block path, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteCrossChunk(t *testing.T) {
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: world.Dimensions{Width: 4, Depth: 3, Height: 4},
	}
	navigator, _, generator := newNavigatorWithRegion(t, region)

	chunkA := newChunkForRegion(t, region, world.ChunkCoord{X: 0, Y: 0})
	chunkB := newChunkForRegion(t, region, world.ChunkCoord{X: 1, Y: 0})
	addFloor(chunkA, 0)
	addFloor(chunkB, 0)
	generator.setChunk(chunkA.Key, chunkA)
	generator.setChunk(chunkB.Key, chunkB)

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: region.ChunkDimension.Width + 1, Y: 1, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path across chunk boundary")
	}
	if path[0] != start || path[len(path)-1] != goal {
		t.Fatalf("unexpected path endpoints %v", path)
	}

	crossedBoundary := false
	boundaryX := region.ChunkDimension.Width
	for _, step := range path {
		if step.X == boundaryX {
			crossedBoundary = true
			break
		}
	}
	if !crossedBoundary {
		t.Fatalf("expected path to cross chunk boundary, got %v", path)
	}
}

func TestBlockNavigatorFlyingRouteOverCanyon(t *testing.T) {
	dims := world.Dimensions{Width: 6, Depth: 1, Height: 5}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	for x := 2; x <= 3; x++ {
		chunk.ClearLocalBlock(x, 0, 0)
	}

	start := world.BlockCoord{X: 0, Y: 0, Z: 2}
	goal := world.BlockCoord{X: 5, Y: 0, Z: 2}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected ground unit to fail across canyon, got %v", path)
	}

	flyingPath := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying))
	if len(flyingPath) == 0 {
		t.Fatalf("expected flying unit to cross canyon")
	}
	if flyingPath[0] != start || flyingPath[len(flyingPath)-1] != goal {
		t.Fatalf("unexpected flying path %v", flyingPath)
	}

	crossedGap := false
	for _, step := range flyingPath {
		if step.X >= 2 && step.X <= 3 {
			crossedGap = true
		}
	}
	if !crossedGap {
		t.Fatalf("expected flying path to traverse canyon, got %v", flyingPath)
	}
}

func TestBlockNavigatorStartEqualsGoalReturnsSingleNode(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 3, Height: 3}
	navigator, chunk := newTestNavigator(t, dims)
	addFloor(chunk, 0)

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	path := navigator.FindRoute(context.Background(), start, start, DefaultProfile(ModeGround))
	if len(path) != 1 {
		t.Fatalf("expected single-node path, got %v", path)
	}
	if path[0] != start {
		t.Fatalf("expected start coordinate, got %v", path[0])
	}
}

func TestBlockNavigatorRejectsStartOutsideRegion(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 4, Height: 4}
	navigator, _ := newTestNavigator(t, dims)

	start := world.BlockCoord{X: -1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 1, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected nil path for start outside region, got %v", path)
	}
}

func TestBlockNavigatorRejectsGoalOutsideRegion(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 4, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)
	addFloor(chunk, 0)

	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 10, Y: 1, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected nil path for goal outside region, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteClimbsSingleStep(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	chunk.SetLocalBlock(1, 0, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(2, 0, 1, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 2}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path up single step")
	}

	climbed := false
	for _, step := range path {
		if step.Z > start.Z {
			climbed = true
		}
	}
	if !climbed {
		t.Fatalf("expected path to climb, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteDescendsSingleStep(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	chunk.SetLocalBlock(0, 0, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(1, 0, 0, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 2}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path down single step")
	}

	descended := false
	for _, step := range path {
		if step.Z < start.Z {
			descended = true
		}
	}
	if !descended {
		t.Fatalf("expected path to descend, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteAvoidsWideCanyon(t *testing.T) {
	dims := world.Dimensions{Width: 6, Depth: 3, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	for z := 0; z < 2; z++ {
		chunk.ClearLocalBlock(3, 1, z)
	}

	start := world.BlockCoord{X: 0, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 5, Y: 1, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected detour around canyon")
	}

	detoured := false
	for _, step := range path {
		if step.Y != start.Y {
			detoured = true
		}
		if step.X == 3 && step.Y == 1 {
			t.Fatalf("path should avoid unsupported canyon tile: %v", path)
		}
	}
	if !detoured {
		t.Fatalf("expected path to leave corridor, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteNavigatesCorner(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 4, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	for z := 0; z < 3; z++ {
		chunk.SetLocalBlock(1, 1, z, world.Block{Type: world.BlockSolid})
		chunk.SetLocalBlock(2, 2, z, world.Block{Type: world.BlockSolid})
	}

	start := world.BlockCoord{X: 0, Y: 2, Z: 1}
	goal := world.BlockCoord{X: 3, Y: 0, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround))
	if len(path) == 0 {
		t.Fatalf("expected path around corner")
	}

	sawYChange := false
	sawXChange := false
	prev := path[0]
	for _, step := range path[1:] {
		if step.Y != prev.Y {
			sawYChange = true
		}
		if step.X != prev.X {
			sawXChange = true
		}
		prev = step
	}
	if !sawYChange || !sawXChange {
		t.Fatalf("expected path to include both horizontal and vertical movement, got %v", path)
	}
}

func TestBlockNavigatorFlyingRouteRespectsClearance(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying)); path != nil {
		t.Fatalf("expected flying profile to respect clearance, got %v", path)
	}
}

func TestBlockNavigatorFlyingRouteHonorsMaxClimb(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 1, Height: 5}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 2}
	goal := world.BlockCoord{X: 3, Y: 0, Z: 2}

	limited := DefaultProfile(ModeFlying)
	limited.MaxClimb = 0

	if path := navigator.FindRoute(context.Background(), start, goal, limited); path != nil {
		t.Fatalf("expected limited climb to block path, got %v", path)
	}

	defaultPath := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying))
	if len(defaultPath) == 0 {
		t.Fatalf("expected default flying profile to find route")
	}
}

func TestBlockNavigatorFlyingRouteCrossChunk(t *testing.T) {
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: world.Dimensions{Width: 4, Depth: 1, Height: 5},
	}
	navigator, _, generator := newNavigatorWithRegion(t, region)

	chunkA := newChunkForRegion(t, region, world.ChunkCoord{X: 0, Y: 0})
	chunkB := newChunkForRegion(t, region, world.ChunkCoord{X: 1, Y: 0})
	addFloor(chunkA, 0)
	addFloor(chunkB, 0)
	generator.setChunk(chunkA.Key, chunkA)
	generator.setChunk(chunkB.Key, chunkB)

	start := world.BlockCoord{X: 1, Y: 0, Z: 2}
	goal := world.BlockCoord{X: region.ChunkDimension.Width + 2, Y: 0, Z: 2}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying))
	if len(path) == 0 {
		t.Fatalf("expected flying path across chunks")
	}
	if path[0] != start || path[len(path)-1] != goal {
		t.Fatalf("unexpected flying path %v", path)
	}

	crossed := false
	for _, step := range path {
		if step.X >= region.ChunkDimension.Width {
			crossed = true
		}
	}
	if !crossed {
		t.Fatalf("expected flying path to cross chunk boundary, got %v", path)
	}
}

func TestBlockNavigatorUndergroundRouteBlockedBySolid(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)

	addFloor(chunk, 0)
	chunk.SetLocalBlock(1, 0, 1, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(1, 0, 2, world.Block{Type: world.BlockSolid})
	chunk.SetLocalBlock(1, 0, 3, world.Block{Type: world.BlockSolid})

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeUnderground)); path != nil {
		t.Fatalf("expected solid block to stop underground path, got %v", path)
	}
}

func TestBlockNavigatorUndergroundRouteCrossChunkThroughMineral(t *testing.T) {
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: world.Dimensions{Width: 3, Depth: 1, Height: 4},
	}
	navigator, _, generator := newNavigatorWithRegion(t, region)

	chunkA := newChunkForRegion(t, region, world.ChunkCoord{X: 0, Y: 0})
	chunkB := newChunkForRegion(t, region, world.ChunkCoord{X: 1, Y: 0})
	addFloor(chunkA, 0)
	addFloor(chunkB, 0)
	chunkB.SetLocalBlock(0, 0, 1, world.Block{Type: world.BlockMineral})
	chunkB.SetLocalBlock(0, 0, 2, world.Block{Type: world.BlockSolid})
	chunkB.SetLocalBlock(0, 0, 3, world.Block{Type: world.BlockSolid})
	generator.setChunk(chunkA.Key, chunkA)
	generator.setChunk(chunkB.Key, chunkB)

	start := world.BlockCoord{X: 1, Y: 0, Z: 1}
	goal := world.BlockCoord{X: region.ChunkDimension.Width + 1, Y: 0, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected ground path to fail through mineral, got %v", path)
	}

	tunnelPath := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeUnderground))
	if len(tunnelPath) == 0 {
		t.Fatalf("expected underground path through mineral vein")
	}
	if tunnelPath[0] != start || tunnelPath[len(tunnelPath)-1] != goal {
		t.Fatalf("unexpected underground path %v", tunnelPath)
	}
}

func TestBlockNavigatorGroundRouteNeedsSupportAcrossBoundary(t *testing.T) {
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  2,
		ChunkDimension: world.Dimensions{Width: 3, Depth: 1, Height: 4},
	}
	navigator, _, generator := newNavigatorWithRegion(t, region)

	chunkA := newChunkForRegion(t, region, world.ChunkCoord{X: 0, Y: 0})
	chunkB := newChunkForRegion(t, region, world.ChunkCoord{X: 1, Y: 0})
	addFloor(chunkA, 0)
	addFloor(chunkB, 0)
	chunkB.ClearLocalBlock(0, 0, 0)
	generator.setChunk(chunkA.Key, chunkA)
	generator.setChunk(chunkB.Key, chunkB)

	start := world.BlockCoord{X: 1, Y: 0, Z: 1}
	goal := world.BlockCoord{X: region.ChunkDimension.Width, Y: 0, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected missing support to block path, got %v", path)
	}
}

func TestBlockNavigatorFindRouteCancelledContext(t *testing.T) {
	dims := world.Dimensions{Width: 4, Depth: 2, Height: 4}
	navigator, chunk := newTestNavigator(t, dims)
	addFloor(chunk, 0)

	start := world.BlockCoord{X: 0, Y: 0, Z: 1}
	goal := world.BlockCoord{X: 3, Y: 1, Z: 1}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if path := navigator.FindRoute(ctx, start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected cancelled context to yield no path, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteFailsWithNilWorld(t *testing.T) {
	region := world.ServerRegion{
		Origin:         world.ChunkCoord{X: 0, Y: 0},
		ChunksPerAxis:  1,
		ChunkDimension: world.Dimensions{Width: 4, Depth: 4, Height: 4},
	}

	navigator := &BlockNavigator{region: region, world: nil}
	start := world.BlockCoord{X: 1, Y: 1, Z: 1}
	goal := world.BlockCoord{X: 2, Y: 1, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected nil world to produce no path, got %v", path)
	}
}

func TestBlockNavigatorFlyingRoutePerformsDropWithinLimit(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 5}
	navigator, chunk := newTestNavigator(t, dims)
	addFloor(chunk, 0)

	start := world.BlockCoord{X: 0, Y: 0, Z: 3}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeFlying))
	if len(path) == 0 {
		t.Fatalf("expected flying path to descend within limit")
	}

	descended := false
	for _, step := range path {
		if step.Z < start.Z {
			descended = true
		}
	}
	if !descended {
		t.Fatalf("expected flying path to descend, got %v", path)
	}
}

func TestBlockNavigatorGroundRouteCannotStartOnWorldFloor(t *testing.T) {
	dims := world.Dimensions{Width: 3, Depth: 1, Height: 3}
	navigator, chunk := newTestNavigator(t, dims)
	addFloor(chunk, 0)

	start := world.BlockCoord{X: 0, Y: 0, Z: 0}
	goal := world.BlockCoord{X: 2, Y: 0, Z: 1}

	if path := navigator.FindRoute(context.Background(), start, goal, DefaultProfile(ModeGround)); path != nil {
		t.Fatalf("expected start on world floor to be invalid, got %v", path)
	}
}

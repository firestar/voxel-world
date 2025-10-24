package ai

import (
	"testing"
	"time"

	"chunkserver/internal/config"
	"chunkserver/internal/entities"
	"chunkserver/internal/pathfinding"
	"chunkserver/internal/world"
)

func TestCoordinatorAssignsSquadsAndPlans(t *testing.T) {
	cfg := config.Default()
	region := world.NewServerRegion(cfg)
	mgr := entities.NewManager(cfg.Server.ID)
	nav := pathfinding.NewBlockNavigator(region, nil)
	baseChunk := world.ChunkCoord{X: region.Origin.X, Y: region.Origin.Y + region.ChunksPerAxis - 1}
	lookup := func(chunk world.ChunkCoord) (NeighborOwnership, bool) {
		if chunk == (world.ChunkCoord{X: baseChunk.X, Y: baseChunk.Y + 1}) {
			return NeighborOwnership{
				ServerID:     "remote-north",
				Endpoint:     "127.0.0.1:19001",
				RegionOrigin: world.ChunkCoord{X: baseChunk.X, Y: baseChunk.Y + 1},
				RegionSize:   cfg.Chunk.ChunksPerAxis,
			}, true
		}
		return NeighborOwnership{}, false
	}
	coord := NewCoordinator(region, mgr, nav, lookup)

	baseX := float64(baseChunk.X * cfg.Chunk.Width)
	baseY := float64(baseChunk.Y * cfg.Chunk.Depth)
	makeUnit := func(id string, offset entities.Vec3, caps entities.Capabilities) *entities.Entity {
		return &entities.Entity{
			ID:   entities.ID(id),
			Kind: entities.KindUnit,
			Chunk: entities.ChunkMembership{
				ServerID: cfg.Server.ID,
				Chunk:    baseChunk,
			},
			Position: entities.Vec3{
				X: baseX + offset.X,
				Y: baseY + offset.Y,
				Z: offset.Z,
			},
			Capabilities: caps,
			Stats:        entities.Stats{MaxHP: 100, CurrentHP: 100},
		}
	}

	ground := makeUnit("unit-ground", entities.Vec3{X: 10, Y: 12, Z: 2}, entities.Capabilities{})
	air := makeUnit("unit-air", entities.Vec3{X: 14, Y: 16, Z: 8}, entities.Capabilities{CanFly: true})
	builder := makeUnit("unit-builder", entities.Vec3{X: 8, Y: 10, Z: 1}, entities.Capabilities{CanDig: true})

	for _, ent := range []*entities.Entity{ground, air, builder} {
		if err := mgr.Add(ent); err != nil {
			t.Fatalf("add entity %s: %v", ent.ID, err)
		}
	}

	coord.Tick(33 * time.Millisecond)

	if role, ok := ground.Attribute("ai_squad_role"); !ok || int(role) != roleCode(SquadRoleAssault) {
		t.Fatalf("ground unit squad role mismatch: %v", role)
	}
	if role, ok := air.Attribute("ai_squad_role"); !ok || int(role) != roleCode(SquadRoleSupport) {
		t.Fatalf("air unit squad role mismatch: %v", role)
	}
	if role, ok := builder.Attribute("ai_squad_role"); !ok || int(role) != roleCode(SquadRoleBuilder) {
		t.Fatalf("builder unit squad role mismatch: %v", role)
	}

	if _, ok := builder.Attribute("ai_construction_progress"); !ok {
		t.Fatalf("builder lacks construction progress attribute")
	}

	plan, ok := coord.Plan("builder-plan")
	if !ok {
		t.Fatalf("expected construction plan for builder squad")
	}
	if len(plan.Steps) == 0 {
		t.Fatalf("plan missing steps")
	}
	if plan.Progress <= 0 {
		t.Fatalf("plan progress not computed: %v", plan.Progress)
	}

	// Ensure at least one step references the remote owner for cross-chunk coordination.
	remoteCount := 0
	for _, step := range plan.Steps {
		if step.RemoteOwner != "" {
			remoteCount++
		}
	}
	if remoteCount == 0 {
		t.Fatalf("expected remote coordination steps, found none")
	}

	if dist, ok := ground.Attribute("ai_target_distance"); !ok || dist <= 0 {
		t.Fatalf("ground unit missing movement intent")
	}
	if remoteHint, ok := air.Attribute("ai_remote_server_hint"); !ok || remoteHint == 0 {
		t.Fatalf("air unit missing remote coordination hint")
	}
}

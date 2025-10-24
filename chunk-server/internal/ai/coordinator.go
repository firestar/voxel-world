package ai

import (
	"fmt"
	"hash/crc32"
	"math"
	"sort"
	"sync"
	"time"

	"chunkserver/internal/entities"
	"chunkserver/internal/pathfinding"
	"chunkserver/internal/world"
)

// SquadRole defines the tactical responsibility of a squad.
type SquadRole string

const (
	SquadRoleAssault SquadRole = "assault"
	SquadRoleSupport SquadRole = "support"
	SquadRoleBuilder SquadRole = "builder"
)

// ObjectiveKind represents the purpose a squad is pursuing.
type ObjectiveKind string

const (
	ObjectiveHold   ObjectiveKind = "hold"
	ObjectiveAttack ObjectiveKind = "attack"
	ObjectiveBuild  ObjectiveKind = "build"
	ObjectiveEscort ObjectiveKind = "escort"
)

// Objective describes the high-level intent for a squad.
type Objective struct {
	Kind        ObjectiveKind
	TargetChunk world.ChunkCoord
	TargetBlock world.BlockCoord
	Description string
}

// SquadMember tracks slot assignment metadata.
type SquadMember struct {
	EntityID   entities.ID
	SlotIndex  int
	LastUpdate time.Time
}

// Squad represents a tactical grouping of units.
type Squad struct {
	ID        string
	Role      SquadRole
	Formation Formation
	Objective Objective
	Members   map[entities.ID]*SquadMember
}

// SquadSnapshot is a read-only view of a squad.
type SquadSnapshot struct {
	ID        string
	Role      SquadRole
	Formation Formation
	Objective Objective
	Members   []SquadMember
}

// Coordinator orchestrates squads, formations, and construction plans.
type Coordinator struct {
	region    world.ServerRegion
	entities  *entities.Manager
	navigator *pathfinding.BlockNavigator
	lookup    NeighborLookup

	mu     sync.RWMutex
	squads map[string]*Squad
	plans  map[string]*ConstructionPlan
}

// NewCoordinator constructs a new AI coordinator.
func NewCoordinator(region world.ServerRegion, mgr *entities.Manager, nav *pathfinding.BlockNavigator, lookup NeighborLookup) *Coordinator {
	return &Coordinator{
		region:    region,
		entities:  mgr,
		navigator: nav,
		lookup:    lookup,
		squads:    make(map[string]*Squad),
		plans:     make(map[string]*ConstructionPlan),
	}
}

// Tick evaluates squads and updates entity intents.
func (c *Coordinator) Tick(delta time.Duration) {
	if c == nil || c.entities == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rebuildSquads()
	c.updateFormations(delta)
	c.updateConstructionPlans(delta)
}

// SquadSnapshot returns a copy of the requested squad state.
func (c *Coordinator) SquadSnapshot(id string) (SquadSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	squad, ok := c.squads[id]
	if !ok {
		return SquadSnapshot{}, false
	}
	snapshot := SquadSnapshot{
		ID:        squad.ID,
		Role:      squad.Role,
		Formation: squad.Formation,
		Objective: squad.Objective,
	}
	if len(squad.Members) > 0 {
		snapshot.Members = make([]SquadMember, 0, len(squad.Members))
		for _, member := range squad.Members {
			snapshot.Members = append(snapshot.Members, *member)
		}
		sort.Slice(snapshot.Members, func(i, j int) bool {
			return snapshot.Members[i].SlotIndex < snapshot.Members[j].SlotIndex
		})
	}
	return snapshot, true
}

// Plan returns a copy of the named construction plan.
func (c *Coordinator) Plan(id string) (ConstructionPlan, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	plan, ok := c.plans[id]
	if !ok {
		return ConstructionPlan{}, false
	}
	return plan.Clone(), true
}

func (c *Coordinator) rebuildSquads() {
	chunks := c.entities.ActiveChunks()
	seen := make(map[entities.ID]struct{})
	for _, coord := range chunks {
		list := c.entities.MutableByChunk(coord)
		if len(list) == 0 {
			continue
		}
		for _, ent := range list {
			if ent == nil {
				continue
			}
			if ent.Kind != entities.KindUnit {
				continue
			}
			role := classifyRole(ent)
			squad := c.ensureSquad(role)
			member := squad.Members[ent.ID]
			if member == nil {
				member = &SquadMember{EntityID: ent.ID}
				squad.Members[ent.ID] = member
			}
			member.LastUpdate = time.Now()
			seen[ent.ID] = struct{}{}
			ent.SetAttribute("ai_squad_role", float64(roleCode(role)))
		}
	}
	for _, squad := range c.squads {
		for id := range squad.Members {
			if _, ok := seen[id]; !ok {
				delete(squad.Members, id)
			}
		}
	}
}

func (c *Coordinator) ensureSquad(role SquadRole) *Squad {
	id := string(role)
	if squad, ok := c.squads[id]; ok {
		return squad
	}
	formation := Formation{Type: formationForRole(role), Spacing: spacingForRole(role)}
	objective := Objective{Kind: objectiveForRole(role)}
	squad := &Squad{
		ID:        id,
		Role:      role,
		Formation: formation,
		Objective: objective,
		Members:   make(map[entities.ID]*SquadMember),
	}
	c.squads[id] = squad
	return squad
}

func (c *Coordinator) updateFormations(delta time.Duration) {
	for _, squad := range c.squads {
		if len(squad.Members) == 0 {
			continue
		}
		members := sortedMembers(squad.Members)
		for _, member := range members {
			member.SlotIndex = -1
		}
		c.assignSlots(squad, members)
		c.applyObjectives(squad)
		c.driveMembers(squad, members, delta)
	}
}

func sortedMembers(members map[entities.ID]*SquadMember) []*SquadMember {
	out := make([]*SquadMember, 0, len(members))
	for _, member := range members {
		out = append(out, member)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SlotIndex == out[j].SlotIndex {
			return string(out[i].EntityID) < string(out[j].EntityID)
		}
		return out[i].SlotIndex < out[j].SlotIndex
	})
	return out
}

func (c *Coordinator) assignSlots(squad *Squad, members []*SquadMember) {
	if len(members) == 0 {
		return
	}
	// Determine consistent ordering by entity ID.
	sort.Slice(members, func(i, j int) bool {
		return string(members[i].EntityID) < string(members[j].EntityID)
	})
	for idx, member := range members {
		member.SlotIndex = idx
	}
}

func (c *Coordinator) applyObjectives(squad *Squad) {
	if len(squad.Members) == 0 {
		return
	}
	// Determine anchor based on average member position.
	var sumX, sumY, sumZ float64
	count := 0.0
	for _, member := range squad.Members {
		ent, ok := c.entities.Entity(member.EntityID)
		if !ok {
			continue
		}
		pos := ent.PositionVec()
		sumX += pos.X
		sumY += pos.Y
		sumZ += pos.Z
		count++
	}
	if count == 0 {
		return
	}
	avg := entities.Vec3{X: sumX / count, Y: sumY / count, Z: sumZ / count}
	anchor := world.BlockCoord{X: int(math.Round(avg.X)), Y: int(math.Round(avg.Y)), Z: int(math.Round(avg.Z))}
	squad.Formation.Anchor = anchor
	squad.Formation.Spacing = spacingForRole(squad.Role)
	// Determine facing towards objective chunk.
	anchorChunk, _ := c.region.LocateBlock(anchor)
	targetChunk := anchorChunk
	switch squad.Role {
	case SquadRoleAssault:
		targetChunk = world.ChunkCoord{X: anchorChunk.X + 1, Y: anchorChunk.Y}
	case SquadRoleSupport:
		targetChunk = world.ChunkCoord{X: anchorChunk.X, Y: anchorChunk.Y + 1}
	case SquadRoleBuilder:
		targetChunk = anchorChunk
	}
	squad.Objective.TargetChunk = targetChunk
	// Aim slightly ahead in the target chunk.
	targetBlock := anchor
	targetBlock.X += int(squad.Formation.Spacing)
	targetBlock.Y += int(squad.Formation.Spacing)
	squad.Objective.TargetBlock = targetBlock
	squad.Formation.Facing = math.Atan2(float64(targetBlock.Y-anchor.Y), float64(targetBlock.X-anchor.X))
	if squad.Role == SquadRoleBuilder {
		squad.Objective.Kind = ObjectiveBuild
	}
	if !c.region.ContainsGlobalChunk(targetChunk) && c.lookup != nil {
		if owner, ok := c.lookup(targetChunk); ok {
			squad.Objective.Description = fmt.Sprintf("coordinate with %s", owner.ServerID)
		} else {
			squad.Objective.Description = "secure frontier objective"
		}
	} else {
		switch squad.Role {
		case SquadRoleAssault:
			squad.Objective.Description = "press the frontline"
		case SquadRoleSupport:
			squad.Objective.Description = "screen flanks for allies"
		case SquadRoleBuilder:
			squad.Objective.Description = "raise frontier outpost"
		}
	}
}

func (c *Coordinator) driveMembers(squad *Squad, members []*SquadMember, delta time.Duration) {
	for _, member := range members {
		ent, ok := c.entities.Entity(member.EntityID)
		if !ok {
			continue
		}
		slotPos := squad.Formation.SlotPosition(member.SlotIndex)
		// Align vertical position with current entity height to avoid oscillations.
		current := ent.PositionVec()
		slotVec := entities.Vec3{
			X: float64(slotPos.X) + 0.5,
			Y: float64(slotPos.Y) + 0.5,
			Z: current.Z,
		}
		dx := slotVec.X - current.X
		dy := slotVec.Y - current.Y
		dz := slotVec.Z - current.Z
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
		ent.SetAttribute("ai_target_chunk_x", float64(squad.Objective.TargetChunk.X))
		ent.SetAttribute("ai_target_chunk_y", float64(squad.Objective.TargetChunk.Y))
		ent.SetAttribute("ai_target_distance", distance)
		ent.SetAttribute("ai_formation_index", float64(member.SlotIndex))
		ent.SetAttribute("ai_squad_spacing", squad.Formation.Spacing)
		ent.SetAttribute("ai_squad_facing", squad.Formation.Facing)
		ent.SetAttribute("ai_objective_kind", float64(objectiveCode(squad.Objective.Kind)))
		ent.SetAttribute("ai_objective_x", float64(squad.Objective.TargetBlock.X))
		ent.SetAttribute("ai_objective_y", float64(squad.Objective.TargetBlock.Y))
		ent.SetAttribute("ai_objective_z", float64(squad.Objective.TargetBlock.Z))
		targetChunk, local := c.region.LocateBlock(slotPos)
		remoteChunk := targetChunk
		remote := !local
		if !remote && !c.region.ContainsGlobalChunk(squad.Objective.TargetChunk) {
			remote = true
			remoteChunk = squad.Objective.TargetChunk
		}
		if !remote {
			ent.SetAttribute("ai_remote_chunk_x", 0)
			ent.SetAttribute("ai_remote_chunk_y", 0)
			ent.SetAttribute("ai_remote_server_hint", 0)
		} else if c.lookup != nil {
			if owner, ok := c.lookup(remoteChunk); ok {
				ent.SetAttribute("ai_remote_chunk_x", float64(remoteChunk.X))
				ent.SetAttribute("ai_remote_chunk_y", float64(remoteChunk.Y))
				ent.SetAttribute("ai_remote_server_hint", float64(crc32.ChecksumIEEE([]byte(owner.ServerID))))
			}
		}
		speed := speedForRole(squad.Role, ent)
		if distance < 0.25 {
			ent.SetVelocity(entities.Vec3{})
			continue
		}
		if distance > 0 {
			scale := speed / distance
			ent.SetVelocity(entities.Vec3{X: dx * scale, Y: dy * scale, Z: dz * scale})
		}
	}
}

func (c *Coordinator) updateConstructionPlans(delta time.Duration) {
	// Currently focus on builder squad coordination.
	builder, ok := c.squads[string(SquadRoleBuilder)]
	if !ok || len(builder.Members) == 0 {
		return
	}
	planID := builder.ID + "-plan"
	plan := c.plans[planID]
	if plan == nil {
		plan = &ConstructionPlan{
			ID:            planID,
			Blueprint:     "frontier_outpost",
			AssignedSquad: builder.ID,
			Required: map[string]int{
				"steel":       120,
				"concrete":    80,
				"electronics": 40,
			},
		}
		c.plans[planID] = plan
	}
	plan.AssignedSquad = builder.ID
	plan.Anchor = builder.Formation.Anchor
	plan.UpdateCoverage(c.region, plan.Anchor, 1, c.lookup)
	active := make(map[world.ChunkCoord]int)
	for _, member := range builder.Members {
		ent, ok := c.entities.Entity(member.EntityID)
		if !ok {
			continue
		}
		pos := ent.PositionVec()
		block := world.BlockCoord{X: int(math.Round(pos.X)), Y: int(math.Round(pos.Y)), Z: int(math.Round(pos.Z))}
		chunk, _ := c.region.LocateBlock(block)
		active[chunk]++
		ent.SetAttribute("ai_construction_anchor_x", float64(plan.Anchor.X))
		ent.SetAttribute("ai_construction_anchor_y", float64(plan.Anchor.Y))
		ent.SetAttribute("ai_construction_span_min_x", float64(plan.ChunkSpan.Min.X))
		ent.SetAttribute("ai_construction_span_min_y", float64(plan.ChunkSpan.Min.Y))
		ent.SetAttribute("ai_construction_span_max_x", float64(plan.ChunkSpan.Max.X))
		ent.SetAttribute("ai_construction_span_max_y", float64(plan.ChunkSpan.Max.Y))
	}
	plan.UpdateProgress(active)
	for _, member := range builder.Members {
		ent, ok := c.entities.Entity(member.EntityID)
		if !ok {
			continue
		}
		ent.SetAttribute("ai_construction_progress", plan.Progress)
	}
	_ = delta
}

func classifyRole(ent *entities.Entity) SquadRole {
	if ent == nil {
		return SquadRoleAssault
	}
	if ent.Capabilities.CanDig {
		return SquadRoleBuilder
	}
	if ent.Capabilities.CanFly {
		return SquadRoleSupport
	}
	return SquadRoleAssault
}

func formationForRole(role SquadRole) FormationType {
	switch role {
	case SquadRoleSupport:
		return FormationCircle
	case SquadRoleBuilder:
		return FormationColumn
	default:
		return FormationLine
	}
}

func spacingForRole(role SquadRole) float64 {
	switch role {
	case SquadRoleSupport:
		return 5
	case SquadRoleBuilder:
		return 4
	default:
		return 3
	}
}

func speedForRole(role SquadRole, ent *entities.Entity) float64 {
	base := 6.0
	switch role {
	case SquadRoleSupport:
		base = 7.5
	case SquadRoleBuilder:
		base = 4.5
	}
	if ent != nil && ent.Capabilities.CanFly {
		base += 1.5
	}
	return base
}

func objectiveForRole(role SquadRole) ObjectiveKind {
	switch role {
	case SquadRoleBuilder:
		return ObjectiveBuild
	case SquadRoleSupport:
		return ObjectiveEscort
	default:
		return ObjectiveAttack
	}
}

func roleCode(role SquadRole) int {
	switch role {
	case SquadRoleSupport:
		return 2
	case SquadRoleBuilder:
		return 3
	default:
		return 1
	}
}

func objectiveCode(kind ObjectiveKind) int {
	switch kind {
	case ObjectiveHold:
		return 1
	case ObjectiveBuild:
		return 2
	case ObjectiveEscort:
		return 3
	case ObjectiveAttack:
		fallthrough
	default:
		return 4
	}
}

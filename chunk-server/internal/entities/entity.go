package entities

import (
	"math"
	"sync"
	"time"

	"chunkserver/internal/world"
)

type ID string

type Kind string

const (
	KindUnit       Kind = "unit"
	KindProjectile Kind = "projectile"
	KindStructure  Kind = "structure"
	KindFactory    Kind = "factory"
)

// Vec3 represents positions scaled to world blocks (float precision supports 1/20th block entities).
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

type Rotation struct {
	Yaw   float64
	Pitch float64
	Roll  float64
}

type ChunkMembership struct {
	ServerID string
	Chunk    world.ChunkCoord
}

type Entity struct {
	mu sync.RWMutex

	ID         ID
	Kind       Kind
	Name       string
	Chunk      ChunkMembership
	Position   Vec3
	Velocity   Vec3
	Acceleration Vec3
	Orientation Rotation
	Blocks     []EntityBlock
	Stats      Stats
	Capabilities Capabilities
	Attributes map[string]float64

	LastTick time.Time
	Dirty    bool
	Dying    bool
}

type PhysicsParams struct {
	Gravity         float64
	AirDrag         float64
	GroundFriction  float64
	MaxFallSpeed    float64
	SupportsGravity bool
}

type ProjectileParams struct {
	Lifetime    time.Duration
	SpawnTime   time.Time
	ExplosiveYield float64
	ImpactRadius   float64
	DamageFalloff  float64
}

type EntityBlockRole string

const (
	BlockRoleStructure EntityBlockRole = "structure"
	BlockRoleThruster  EntityBlockRole = "thruster"
	BlockRoleWeapon    EntityBlockRole = "weapon"
	BlockRoleFactory   EntityBlockRole = "factory"
	BlockRolePower     EntityBlockRole = "power"
	BlockRoleGas       EntityBlockRole = "gas"
)

type EntityBlock struct {
	// Offset measured in entity voxels (1 voxel = 1/20 world block).
	Offset Vec3
	VoxelSize float64
	Block     world.Block
	Role      EntityBlockRole
}

type Stats struct {
	MaxHP       float64
	CurrentHP   float64
	BlockHP     []float64
	RepairRate  float64 // blocks per second
	Mass        float64
}

type Capabilities struct {
	CanFly             bool
	CanDig             bool
	CanProduceUnits    bool
	ProjectileVelocity float64
	ProjectileArc      bool
	UndergroundClearance float64
}

func (e *Entity) Snapshot() Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	copyEntity := *e
	if e.Blocks != nil {
		copyEntity.Blocks = append([]EntityBlock(nil), e.Blocks...)
	}
	if e.Stats.BlockHP != nil {
		copyEntity.Stats.BlockHP = append([]float64(nil), e.Stats.BlockHP...)
	}
	if e.Attributes != nil {
		copyEntity.Attributes = make(map[string]float64, len(e.Attributes))
		for k, v := range e.Attributes {
			copyEntity.Attributes[k] = v
		}
	}
	return copyEntity
}

func (e *Entity) UpdateChunk(serverID string, coord world.ChunkCoord) {
	e.mu.Lock()
	e.Chunk.ServerID = serverID
	e.Chunk.Chunk = coord
	e.Dirty = true
	e.mu.Unlock()
}

func (e *Entity) Advance(delta time.Duration) {
	e.mu.Lock()
	seconds := delta.Seconds()
	e.Position.X += e.Velocity.X * seconds
	e.Position.Y += e.Velocity.Y * seconds
	e.Position.Z += e.Velocity.Z * seconds
	e.LastTick = time.Now()
	e.Dirty = true
	e.mu.Unlock()
}

func (e *Entity) ApplyDamage(amount float64) {
	if amount <= 0 {
		return
	}
	e.mu.Lock()
	e.Stats.CurrentHP -= amount
	e.Dirty = true
	if e.Stats.CurrentHP < 0 {
		e.Stats.CurrentHP = 0
		e.Dying = true
	}
	e.mu.Unlock()
}

func (e *Entity) HealBlocks(blocksPerSecond float64, delta time.Duration) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.Stats.BlockHP) == 0 || blocksPerSecond <= 0 {
		return 0
	}

	toRepair := blocksPerSecond * delta.Seconds()
	repaired := 0.0
	for i := range e.Stats.BlockHP {
		if toRepair <= 0 {
			break
		}
		current := e.Stats.BlockHP[i]
		max := e.Blocks[i].Block.MaxHitPoints
		if current >= max {
			continue
		}
		needed := max - current
		restore := needed
		if restore > toRepair {
			restore = toRepair
		}
		e.Stats.BlockHP[i] += restore
		e.Stats.CurrentHP += restore
		toRepair -= restore
		repaired += restore
	}
	if e.Stats.CurrentHP > e.Stats.MaxHP {
		e.Stats.CurrentHP = e.Stats.MaxHP
	}
	if repaired > 0 {
		e.Dirty = true
	}
	return repaired
}

func (e *Entity) ApplyGravity(params PhysicsParams, delta time.Duration) {
	if params.Gravity == 0 || !params.SupportsGravity {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	seconds := delta.Seconds()
	e.Velocity.Z -= params.Gravity * seconds
	if params.MaxFallSpeed > 0 && e.Velocity.Z < -params.MaxFallSpeed {
		e.Velocity.Z = -params.MaxFallSpeed
	}
	e.Dirty = true
}

func (e *Entity) ApplyDrag(params PhysicsParams, delta time.Duration) {
	if params.AirDrag <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	factor := math.Exp(-params.AirDrag * delta.Seconds())
	e.Velocity.X *= factor
	e.Velocity.Y *= factor
	e.Velocity.Z *= factor
	e.Dirty = true
}

func (e *Entity) ApplyGroundFriction(coeff float64, delta time.Duration) {
	if coeff <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	factor := math.Exp(-coeff * delta.Seconds())
	e.Velocity.X *= factor
	e.Velocity.Y *= factor
	e.Dirty = true
}

func (e *Entity) MarkClean() {
	e.mu.Lock()
	e.Dirty = false
	e.mu.Unlock()
}

func (e *Entity) IsDirty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Dirty
}

func (e *Entity) SetPosition(pos Vec3) {
	e.mu.Lock()
	e.Position = pos
	e.Dirty = true
	e.mu.Unlock()
}

func (e *Entity) SetVelocity(vel Vec3) {
	e.mu.Lock()
	e.Velocity = vel
	e.Dirty = true
	e.mu.Unlock()
}

func (e *Entity) PositionVec() Vec3 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Position
}

func (e *Entity) ClampZ(min float64) {
	e.mu.Lock()
	if e.Position.Z < min {
		e.Position.Z = min
		if e.Velocity.Z < 0 {
			e.Velocity.Z = 0
		}
		e.Dirty = true
	}
	e.mu.Unlock()
}

func (e *Entity) FlagCollapse() {
	e.mu.Lock()
	e.Dying = true
	e.Dirty = true
	e.mu.Unlock()
}

func (e *Entity) ReduceAttribute(key string, amount float64) (float64, bool) {
	if amount == 0 {
		return 0, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Attributes == nil {
		return 0, false
	}
	value, ok := e.Attributes[key]
	if !ok {
		return 0, false
	}
	value -= amount
	e.Attributes[key] = value
	e.Dirty = true
	return value, true
}

func (e *Entity) Attribute(key string) (float64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Attributes == nil {
		return 0, false
	}
	value, ok := e.Attributes[key]
	return value, ok
}

func (e *Entity) SetAttribute(key string, value float64) {
	e.mu.Lock()
	if e.Attributes == nil {
		e.Attributes = make(map[string]float64)
	}
	e.Attributes[key] = value
	e.Dirty = true
	e.mu.Unlock()
}

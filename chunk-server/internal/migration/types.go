package migration

import (
	"time"

	"chunkserver/internal/entities"
	"chunkserver/internal/world"
)

type Direction int

const (
	DirectionEast Direction = iota
	DirectionWest
	DirectionNorth
	DirectionSouth
)

type Request struct {
	EntityID       entities.ID
	EntitySnapshot entities.Entity
	TargetChunk    world.ChunkCoord
	TargetServer   string
	TargetEndpoint string
	QueuedAt       time.Time
	Reason         string
	Nonce          uint64
}

type Result struct {
	EntityID       entities.ID
	Success        bool
	Message        string
	TargetServerID string
	TargetChunk    world.ChunkCoord
	CompletedAt    time.Time
}

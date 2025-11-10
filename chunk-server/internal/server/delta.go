package server

import (
	"time"

	"chunkserver/internal/network"
	"chunkserver/internal/world"
)

type deltaAccumulator struct {
	data map[world.ChunkCoord]map[world.BlockCoord]world.BlockChange
}

var deltaPriority = map[world.ChangeReason]int{
	world.ReasonDamage:   1,
	world.ReasonDestroy:  2,
	world.ReasonCollapse: 3,
}

func newDeltaAccumulator() *deltaAccumulator {
	return &deltaAccumulator{
		data: make(map[world.ChunkCoord]map[world.BlockCoord]world.BlockChange),
	}
}

func (d *deltaAccumulator) add(chunk world.ChunkCoord, change world.BlockChange) {
	if d.data == nil {
		d.data = make(map[world.ChunkCoord]map[world.BlockCoord]world.BlockChange)
	}

	byBlock := d.data[chunk]
	if byBlock == nil {
		byBlock = make(map[world.BlockCoord]world.BlockChange)
		d.data[chunk] = byBlock
	}

	if existing, ok := byBlock[change.Coord]; ok {
		if priority(existing.Reason) > priority(change.Reason) {
			return
		}
		if priority(existing.Reason) == priority(change.Reason) {
			change.Before = existing.Before
		}
	}

	byBlock[change.Coord] = change
}

func (d *deltaAccumulator) flush(serverID string, seq *uint64) []network.ChunkDelta {
	if len(d.data) == 0 {
		return nil
	}

	now := time.Now().UTC()
	deltas := make([]network.ChunkDelta, 0, len(d.data))

	for chunk, blocks := range d.data {
		if len(blocks) == 0 {
			continue
		}
		delta := network.ChunkDelta{
			ServerID:  serverID,
			ChunkX:    chunk.X,
			ChunkY:    chunk.Y,
			Seq:       *seq,
			Timestamp: now,
			Blocks:    make([]network.BlockChange, 0, len(blocks)),
		}
		*seq++
		for coord, change := range blocks {
			delta.Blocks = append(delta.Blocks, network.BlockChange{
				X:        coord.X,
				Y:        coord.Y,
				Z:        coord.Z,
				Type:     encodeBlockType(change.After.Type),
				Material: change.After.Material,
				Color:    change.After.Color,
				Texture:  change.After.Texture,
				HP:       change.After.HitPoints,
				MaxHP:    change.After.MaxHitPoints,
				Reason:   encodeChangeReason(change.Reason),
				Light:    change.After.LightEmission,
			})
		}
		deltas = append(deltas, delta)
	}

	d.data = make(map[world.ChunkCoord]map[world.BlockCoord]world.BlockChange)
	return deltas
}

func priority(reason world.ChangeReason) int {
	if v, ok := deltaPriority[reason]; ok {
		return v
	}
	return 0
}

func encodeBlockType(t world.BlockType) network.BlockTypeCode {
	switch t {
	case world.BlockAir:
		return network.BlockTypeAir
	case world.BlockSolid:
		return network.BlockTypeSolid
	case world.BlockUnstable:
		return network.BlockTypeUnstable
	case world.BlockMineral:
		return network.BlockTypeMineral
	case world.BlockExplosive:
		return network.BlockTypeExplosive
	default:
		return network.BlockTypeUnknown
	}
}

func encodeChangeReason(reason world.ChangeReason) network.ChangeReasonCode {
	switch reason {
	case world.ReasonDamage:
		return network.ChangeReasonDamage
	case world.ReasonDestroy:
		return network.ChangeReasonDestroy
	case world.ReasonCollapse:
		return network.ChangeReasonCollapse
	default:
		return network.ChangeReasonUnknown
	}
}

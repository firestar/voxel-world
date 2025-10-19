package world

import (
	"errors"
	"math"
)

const (
	groundSupportForce = 1e6
	hangingPenalty     = 0.72
)

// StabilityReport captures the state of a block after evaluating column stability.
type StabilityReport struct {
	Global        BlockCoord
	LocalZ        int
	Block         Block
	Stable        bool
	Collapsed     bool
	Hanging       bool
	ChainDepth    int
	SupportForce  float64
	RequiredForce float64
}

type columnNode struct {
	block          Block
	initialPresent bool
	present        bool
	collapsed      bool
	stable         bool

	load        float64
	lastSupport float64
	support     float64
	required    float64
	hanging     bool
	nextStable  bool
	chainDepth  int
}

func evaluateColumnStability(chunk *Chunk, localX, localY int) ([]StabilityReport, error) {
	dim := chunk.dimension
	if localX < 0 || localY < 0 || localX >= dim.Width || localY >= dim.Depth {
		return nil, errors.New("column coordinates out of bounds")
	}

	nodes := make([]columnNode, dim.Height)
	for z := 0; z < dim.Height; z++ {
		block, ok := chunk.LocalBlock(localX, localY, z)
		if !ok {
			return nil, errors.New("block coordinates out of bounds")
		}
		present := block.Type != BlockAir
		nodes[z] = columnNode{
			block:          block,
			initialPresent: present,
			present:        present,
		}
	}

	mutated := true
	for mutated {
		mutated = false

		// Pass 1: accumulate load from top to bottom.
		var weightAbove float64
		for z := dim.Height - 1; z >= 0; z-- {
			node := &nodes[z]
			if !node.present {
				node.load = 0
				weightAbove = 0
				continue
			}
			node.load = weightAbove + node.block.Weight
			weightAbove = node.load
		}

		// Pass 2: evaluate support from bottom to top without mutating presence yet.
		var chainDepth int
		chainPenalty := 1.0
		for z := 0; z < dim.Height; z++ {
			node := &nodes[z]
		if !node.present {
			if !node.collapsed {
				node.lastSupport = 0
				node.hanging = false
				node.chainDepth = 0
			}
			node.nextStable = false
				chainDepth = 0
				chainPenalty = 1.0
				continue
			}

			support := node.block.ConnectingForce
			hanging := false

			if z == 0 {
				// Base layer anchored to bedrock.
				support += groundSupportForce
				chainDepth = 0
				chainPenalty = 1.0
			} else {
				below := &nodes[z-1]
				if !below.present {
					hanging = true
					chainDepth++
					chainPenalty *= hangingPenalty
					support *= chainPenalty
				} else {
					chainDepth = 0
					chainPenalty = 1.0
					support = math.Min(support, below.block.ConnectingForce)
					if below.lastSupport > 0 {
						support = math.Min(support, below.lastSupport)
					}
				}
			}

			node.lastSupport = support
			node.hanging = hanging
			node.chainDepth = chainDepth
			node.nextStable = support >= node.load
		}

		// Pass 3: commit collapses determined in this iteration.
		for z := 0; z < dim.Height; z++ {
			node := &nodes[z]
			if !node.present {
				continue
			}
			node.support = node.lastSupport
			node.required = node.load
			if !node.nextStable {
				node.present = false
				if node.initialPresent && !node.collapsed {
					node.collapsed = true
					mutated = true
				}
				node.stable = false
				continue
			}
			node.stable = true
		}
	}

	// Produce reports for blocks that existed or collapsed.
	reports := make([]StabilityReport, 0, dim.Height)
	for z := 0; z < dim.Height; z++ {
		node := nodes[z]
		if !node.initialPresent && !node.collapsed {
			continue
		}

		reports = append(reports, StabilityReport{
			Global: BlockCoord{
				X: chunk.Bounds.Min.X + localX,
				Y: chunk.Bounds.Min.Y + localY,
				Z: chunk.Bounds.Min.Z + z,
			},
			LocalZ:        z,
			Block:         node.block,
			Stable:        node.stable && !node.collapsed,
			Collapsed:     node.collapsed,
			Hanging:       node.hanging,
			ChainDepth:    node.chainDepth,
			SupportForce:  node.support,
			RequiredForce: node.required,
		})
	}

	return reports, nil
}

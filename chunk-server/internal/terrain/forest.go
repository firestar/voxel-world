package terrain

import (
	"math"

	"chunkserver/internal/world"
)

type treeVariant struct {
	name             string
	trunkBlock       world.Block
	branchBlock      world.Block
	leavesBlock      world.Block
	rootBlock        world.Block
	floorBlock       world.Block
	stairBlock       world.Block
	accentBlock      world.Block
	veinBlock        world.Block
	trunkRadius      int
	trunkHeight      int
	canopyBaseOffset int
	canopyHeight     int
	canopyRadius     int
	branchLength     int
	branchThickness  int
	branchLift       int
	branchLevels     []int
	minSpacing       int
	interiorRadius   int
	walkwayInterval  int
	entryHeight      int
	stumpRadius      int
	stumpHeight      int
	rootReach        int
	rootDepth        int
	hasVeins         bool
}

type treePlacement struct {
	localX        int
	localY        int
	surfaceLocalZ int
	globalX       int
	globalY       int
	variant       *treeVariant
	orientation   int
}

func (g *NoiseGenerator) initTreeVariants() {
	trunkCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       320,
		MaxHitPoints:    320,
		ConnectingForce: 240,
		Weight:          18,
	}

	branchCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       180,
		MaxHitPoints:    180,
		ConnectingForce: 160,
		Weight:          9,
	}

	leavesCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       65,
		MaxHitPoints:    65,
		ConnectingForce: 45,
		Weight:          3,
	}

	rootCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       260,
		MaxHitPoints:    260,
		ConnectingForce: 220,
		Weight:          16,
	}

	floorCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       140,
		MaxHitPoints:    140,
		ConnectingForce: 120,
		Weight:          7,
	}

	stairCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       150,
		MaxHitPoints:    150,
		ConnectingForce: 110,
		Weight:          6,
	}

	accentCommon := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       120,
		MaxHitPoints:    120,
		ConnectingForce: 100,
		Weight:          5,
	}

	luminousVein := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       160,
		MaxHitPoints:    160,
		ConnectingForce: 120,
		Weight:          4,
		LightEmission:   9.5,
	}

	g.treeVariants = []treeVariant{
		{
			name:             "skyhall",
			trunkBlock:       withAppearance(trunkCommon, "skyhall_trunk", "#7e5134", "assets/textures/wood_skyhall.png"),
			branchBlock:      withAppearance(branchCommon, "skyhall_branch", "#8f5f3b", "assets/textures/wood_branch.png"),
			leavesBlock:      withAppearance(leavesCommon, "skyhall_leaf", "#2f8f3f", "assets/textures/leaves_skyhall.png"),
			rootBlock:        withAppearance(rootCommon, "skyhall_root", "#5c3823", "assets/textures/root_skyhall.png"),
			floorBlock:       withAppearance(floorCommon, "skyhall_floor", "#c9a86a", "assets/textures/floor_plank.png"),
			stairBlock:       withAppearance(stairCommon, "skyhall_stair", "#d4b37b", "assets/textures/stair_plank.png"),
			accentBlock:      withAppearance(accentCommon, "skyhall_canopy", "#42a95f", "assets/textures/leaves_bright.png"),
			trunkRadius:      5,
			trunkHeight:      28,
			canopyBaseOffset: 12,
			canopyHeight:     12,
			canopyRadius:     9,
			branchLength:     7,
			branchThickness:  1,
			branchLift:       3,
			branchLevels:     []int{12, 20},
			minSpacing:       18,
			interiorRadius:   3,
			walkwayInterval:  6,
			entryHeight:      5,
			stumpRadius:      2,
			stumpHeight:      3,
			rootReach:        5,
			rootDepth:        3,
			hasVeins:         false,
		},
		{
			name:             "spirebloom",
			trunkBlock:       withAppearance(trunkCommon, "spirebloom_trunk", "#6f442c", "assets/textures/wood_spire.png"),
			branchBlock:      withAppearance(branchCommon, "spirebloom_branch", "#7a4b30", "assets/textures/wood_branch.png"),
			leavesBlock:      withAppearance(leavesCommon, "spirebloom_leaf", "#3ca064", "assets/textures/leaves_spire.png"),
			rootBlock:        withAppearance(rootCommon, "spirebloom_root", "#553221", "assets/textures/root_spire.png"),
			floorBlock:       withAppearance(floorCommon, "spirebloom_floor", "#b98b52", "assets/textures/floor_plank.png"),
			stairBlock:       withAppearance(stairCommon, "spirebloom_stair", "#c49a60", "assets/textures/stair_plank.png"),
			accentBlock:      withAppearance(accentCommon, "spirebloom_canopy", "#4bc478", "assets/textures/leaves_bright.png"),
			trunkRadius:      6,
			trunkHeight:      32,
			canopyBaseOffset: 14,
			canopyHeight:     14,
			canopyRadius:     10,
			branchLength:     8,
			branchThickness:  1,
			branchLift:       4,
			branchLevels:     []int{14, 22},
			minSpacing:       22,
			interiorRadius:   4,
			walkwayInterval:  7,
			entryHeight:      6,
			stumpRadius:      3,
			stumpHeight:      4,
			rootReach:        6,
			rootDepth:        4,
			hasVeins:         false,
		},
		{
			name:             "bastion_oak",
			trunkBlock:       withAppearance(trunkCommon, "bastion_trunk", "#845230", "assets/textures/wood_bastion.png"),
			branchBlock:      withAppearance(branchCommon, "bastion_branch", "#8f5b36", "assets/textures/wood_branch.png"),
			leavesBlock:      withAppearance(leavesCommon, "bastion_leaf", "#4a9d58", "assets/textures/leaves_dense.png"),
			rootBlock:        withAppearance(rootCommon, "bastion_root", "#5a3620", "assets/textures/root_bastion.png"),
			floorBlock:       withAppearance(floorCommon, "bastion_floor", "#cc9c63", "assets/textures/floor_plank.png"),
			stairBlock:       withAppearance(stairCommon, "bastion_stair", "#d7aa70", "assets/textures/stair_plank.png"),
			accentBlock:      withAppearance(accentCommon, "bastion_canopy", "#55b26b", "assets/textures/leaves_bright.png"),
			trunkRadius:      6,
			trunkHeight:      30,
			canopyBaseOffset: 11,
			canopyHeight:     13,
			canopyRadius:     11,
			branchLength:     9,
			branchThickness:  2,
			branchLift:       3,
			branchLevels:     []int{10, 18, 24},
			minSpacing:       24,
			interiorRadius:   4,
			walkwayInterval:  6,
			entryHeight:      6,
			stumpRadius:      3,
			stumpHeight:      4,
			rootReach:        7,
			rootDepth:        4,
			hasVeins:         false,
		},
		{
			name:             "luminara",
			trunkBlock:       withAppearance(trunkCommon, "luminara_trunk", "#6d3c2d", "assets/textures/wood_luminara.png"),
			branchBlock:      withAppearance(branchCommon, "luminara_branch", "#7a4433", "assets/textures/wood_branch.png"),
			leavesBlock:      withAppearance(leavesCommon, "luminara_leaf", "#36a87d", "assets/textures/leaves_luminara.png"),
			rootBlock:        withAppearance(rootCommon, "luminara_root", "#4e2d24", "assets/textures/root_luminara.png"),
			floorBlock:       withAppearance(floorCommon, "luminara_floor", "#c29b6c", "assets/textures/floor_plank.png"),
			stairBlock:       withAppearance(stairCommon, "luminara_stair", "#d0a87a", "assets/textures/stair_plank.png"),
			accentBlock:      withAppearance(accentCommon, "luminara_canopy", "#4de5a1", "assets/textures/leaves_bright.png"),
			veinBlock:        withAppearance(luminousVein, "luminara_vein", "#7fffd4", "assets/textures/vein_lumin.png"),
			trunkRadius:      5,
			trunkHeight:      29,
			canopyBaseOffset: 12,
			canopyHeight:     12,
			canopyRadius:     9,
			branchLength:     8,
			branchThickness:  1,
			branchLift:       3,
			branchLevels:     []int{12, 20},
			minSpacing:       20,
			interiorRadius:   3,
			walkwayInterval:  6,
			entryHeight:      5,
			stumpRadius:      2,
			stumpHeight:      3,
			rootReach:        6,
			rootDepth:        3,
			hasVeins:         true,
		},
	}
}

func (g *NoiseGenerator) growForests(buffer *chunkWriteBuffer, bounds world.Bounds, dim world.Dimensions) error {
	if buffer == nil || len(g.treeVariants) == 0 {
		return nil
	}

	placements := make([]treePlacement, 0, 32)
	for localX := 0; localX < dim.Width; localX++ {
		for localY := 0; localY < dim.Depth; localY++ {
			column, ok := buffer.column(localX, localY)
			if !ok || len(column) == 0 {
				continue
			}

			surfaceIdx := columnSurfaceIndex(column)
			if surfaceIdx < 0 {
				continue
			}

			if surfaceIdx >= len(column) {
				continue
			}

			topBlock := column[surfaceIdx]
			if !isTopsoil(topBlock) {
				continue
			}

			globalX := bounds.Min.X + localX
			globalY := bounds.Min.Y + localY

			if !g.isForestCell(globalX, globalY) {
				continue
			}

			variant := g.selectTreeVariant(globalX, globalY)
			if variant == nil {
				continue
			}

			if g.nearChunkEdge(localX, localY, dim, variant) {
				continue
			}

			if g.slopeTooSteep(buffer, dim, localX, localY, surfaceIdx) {
				continue
			}

			if !g.hasVerticalSpace(dim, surfaceIdx, variant) {
				continue
			}

			if !g.checkForestSpacing(placements, variant, globalX, globalY) {
				continue
			}

			seedVal := hash3(globalX, globalY, int(g.seed^0x95ac3f))
			mask := forestMask(globalX, globalY, g.seed)
			probability := treeProbability(globalX, globalY, g.seed)
			density := 0.45 + mask*0.4
			threshold := 0.35 + probability*0.5
			if density < threshold {
				continue
			}

			randomChance := float64(seedVal&0xFFFF) / 0xFFFF
			if randomChance > density {
				continue
			}

			orientation := int((seedVal >> 5) % 4)

			placements = append(placements, treePlacement{
				localX:        localX,
				localY:        localY,
				surfaceLocalZ: surfaceIdx,
				globalX:       globalX,
				globalY:       globalY,
				variant:       variant,
				orientation:   orientation,
			})
		}
	}

	for _, placement := range placements {
		g.buildTree(buffer, bounds, dim, placement)
	}

	buffer.recalculateUsage()
	return nil
}

func (g *NoiseGenerator) buildTree(buffer *chunkWriteBuffer, bounds world.Bounds, dim world.Dimensions, placement treePlacement) {
	variant := placement.variant
	baseLocalZ := placement.surfaceLocalZ + 1

	trunkTop := baseLocalZ + variant.trunkHeight
	canopyStart := baseLocalZ + variant.canopyBaseOffset
	canopyTop := canopyStart + variant.canopyHeight

	if canopyTop >= dim.Height {
		canopyTop = dim.Height - 1
	}

	// Construct trunk and interior cavity.
	for level := 0; level < variant.trunkHeight && baseLocalZ+level < dim.Height; level++ {
		localZ := baseLocalZ + level
		radius := variant.radiusForLevel(level)
		interior := variant.interiorRadius
		if interior < 1 {
			interior = 1
		}

		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(radius)+0.35 {
					continue
				}

				targetX := placement.localX + dx
				targetY := placement.localY + dy

				if !inColumnBounds(dim, targetX, targetY) {
					continue
				}

				if dist <= float64(interior)-0.25 {
					clearBlock(buffer, dim, targetX, targetY, localZ)
					continue
				}

				block := g.blockForPart(variant.trunkBlock, variant.name, "trunk", map[string]any{
					"level": level,
				})
				setBlock(buffer, dim, targetX, targetY, localZ, block)
			}
		}
	}

	// Interior floors for building functionality.
	if variant.walkwayInterval > 0 {
		for level := variant.walkwayInterval; level < variant.trunkHeight-1; level += variant.walkwayInterval {
			localZ := baseLocalZ + level
			for dx := -variant.interiorRadius; dx <= variant.interiorRadius; dx++ {
				for dy := -variant.interiorRadius; dy <= variant.interiorRadius; dy++ {
					dist := math.Sqrt(float64(dx*dx + dy*dy))
					if dist > float64(variant.interiorRadius)+0.1 {
						continue
					}
					if math.Abs(float64(dx))+math.Abs(float64(dy)) < 1 {
						// leave core open for circulation
						clearBlock(buffer, dim, placement.localX+dx, placement.localY+dy, localZ)
						continue
					}
					block := g.blockForPart(variant.floorBlock, variant.name, "floor", map[string]any{
						"level": level,
					})
					setBlock(buffer, dim, placement.localX+dx, placement.localY+dy, localZ, block)
				}
			}
		}
	}

	// Spiral stair / ramp inside trunk.
	g.buildSpiral(buffer, dim, placement, baseLocalZ)

	// Entrance carve-out.
	g.carveEntrance(buffer, dim, placement, baseLocalZ)

	// Reinforce stump and roots.
	g.buildRoots(buffer, dim, placement, baseLocalZ)

	// Branching structures and canopy.
	for _, branchLevel := range variant.branchLevels {
		g.buildBranches(buffer, dim, placement, baseLocalZ+branchLevel)
	}

	g.buildCanopy(buffer, dim, placement, canopyStart, canopyTop)

	if variant.hasVeins {
		g.decorateVeins(buffer, dim, placement, baseLocalZ, trunkTop, canopyTop)
	}
}

func (g *NoiseGenerator) buildSpiral(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, baseLocalZ int) {
	variant := placement.variant
	if variant.stairBlock.Type == "" {
		return
	}

	offsets := []struct{ dx, dy int }{
		{variant.interiorRadius, 0},
		{0, variant.interiorRadius},
		{-variant.interiorRadius, 0},
		{0, -variant.interiorRadius},
	}
	for level := 0; level < variant.trunkHeight && baseLocalZ+level < dim.Height; level++ {
		stepIndex := (placement.orientation + level) % len(offsets)
		pos := offsets[stepIndex]
		targetX := placement.localX + pos.dx
		targetY := placement.localY + pos.dy
		targetZ := baseLocalZ + level
		if !inColumnBounds(dim, targetX, targetY) {
			continue
		}
		block := g.blockForPart(variant.stairBlock, variant.name, "stair", map[string]any{
			"level": level,
		})
		setBlock(buffer, dim, targetX, targetY, targetZ, block)
		clearBlock(buffer, dim, targetX, targetY, targetZ+1)
	}
}

func (g *NoiseGenerator) carveEntrance(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, baseLocalZ int) {
	variant := placement.variant
	dir := orientationVector(placement.orientation)
	if dir.dx == 0 && dir.dy == 0 {
		return
	}
	radius := variant.radiusForLevel(0)
	width := 2
	height := variant.entryHeight
	for h := 0; h < height; h++ {
		levelRadius := variant.radiusForLevel(h)
		for w := -width; w <= width; w++ {
			var tx, ty int
			if dir.dx != 0 {
				tx = placement.localX + dir.dx*(levelRadius) // shell block
				ty = placement.localY + w
			} else {
				tx = placement.localX + w
				ty = placement.localY + dir.dy*(levelRadius)
			}
			tz := baseLocalZ + h
			if inColumnBounds(dim, tx, ty) {
				clearBlock(buffer, dim, tx, ty, tz)
			}
			// carve tunnel inward
			for offset := 1; offset <= radius; offset++ {
				innerX := placement.localX + dir.dx*(levelRadius-offset)
				innerY := placement.localY + dir.dy*(levelRadius-offset)
				if inColumnBounds(dim, innerX, innerY) {
					clearBlock(buffer, dim, innerX, innerY, tz)
				}
			}
		}
	}

	// Entry platform inside base.
	entryZ := baseLocalZ
	for dx := -variant.interiorRadius; dx <= variant.interiorRadius; dx++ {
		for dy := -variant.interiorRadius; dy <= variant.interiorRadius; dy++ {
			if math.Abs(float64(dx))+math.Abs(float64(dy)) > float64(variant.interiorRadius)+0.1 {
				continue
			}
			block := g.blockForPart(variant.floorBlock, variant.name, "entry", nil)
			setBlock(buffer, dim, placement.localX+dx, placement.localY+dy, entryZ-1, block)
		}
	}
}

func (g *NoiseGenerator) buildRoots(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, baseLocalZ int) {
	variant := placement.variant
	surfaceZ := placement.surfaceLocalZ

	// Buttress stump expansions
	for level := 0; level < variant.stumpHeight; level++ {
		radius := variant.radiusForLevel(level)
		localZ := baseLocalZ + level
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(radius)+0.25 {
					continue
				}
				if !inColumnBounds(dim, placement.localX+dx, placement.localY+dy) {
					continue
				}
				block := g.blockForPart(variant.trunkBlock, variant.name, "stump", map[string]any{"level": level})
				setBlock(buffer, dim, placement.localX+dx, placement.localY+dy, localZ, block)
			}
		}
	}

	directions := []struct{ dx, dy int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}}
	for _, dir := range directions {
		for step := 1; step <= variant.rootReach; step++ {
			targetX := placement.localX + dir.dx*step
			targetY := placement.localY + dir.dy*step
			if !inColumnBounds(dim, targetX, targetY) {
				continue
			}
			depth := step / 2
			if depth > variant.rootDepth {
				depth = variant.rootDepth
			}
			targetZ := surfaceZ - depth
			if targetZ < 0 {
				targetZ = 0
			}
			block := g.blockForPart(variant.rootBlock, variant.name, "root", map[string]any{
				"reach": step,
			})
			setBlock(buffer, dim, targetX, targetY, targetZ, block)
			if depth > 0 {
				for fill := targetZ + 1; fill <= surfaceZ; fill++ {
					setBlock(buffer, dim, targetX, targetY, fill, block)
				}
			}
		}
	}
}

func (g *NoiseGenerator) buildBranches(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, baseZ int) {
	variant := placement.variant
	if baseZ >= dim.Height {
		return
	}

	directions := []struct{ dx, dy int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, dir := range directions {
		for step := 1; step <= variant.branchLength; step++ {
			raise := (step * variant.branchLift) / variant.branchLength
			targetZ := baseZ + raise
			if targetZ >= dim.Height {
				break
			}
			centerX := placement.localX + dir.dx*step
			centerY := placement.localY + dir.dy*step
			if !inColumnBounds(dim, centerX, centerY) {
				continue
			}
			for offset := -variant.branchThickness; offset <= variant.branchThickness; offset++ {
				var tx, ty int
				if dir.dx != 0 {
					tx = centerX
					ty = centerY + offset
				} else {
					tx = centerX + offset
					ty = centerY
				}
				if !inColumnBounds(dim, tx, ty) {
					continue
				}
				block := g.blockForPart(variant.branchBlock, variant.name, "branch", map[string]any{
					"span": step,
				})
				setBlock(buffer, dim, tx, ty, targetZ, block)
				clearBlock(buffer, dim, tx, ty, targetZ+1)
				if variant.hasVeins && variant.veinBlock.Type != "" {
					veinBlock := g.blockForPart(variant.veinBlock, variant.name, "vein", map[string]any{
						"span": step,
					})
					placeIfAir(buffer, dim, tx, ty, targetZ-1, veinBlock)
				}
			}
			g.buildLeafCluster(buffer, dim, centerX, centerY, targetZ+1, variant)
		}
	}
}

func (g *NoiseGenerator) buildLeafCluster(buffer *chunkWriteBuffer, dim world.Dimensions, centerX, centerY, centerZ int, variant *treeVariant) {
	radius := 3
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -1; dz <= 2; dz++ {
				tx := centerX + dx
				ty := centerY + dy
				tz := centerZ + dz
				if tz >= dim.Height || tz < 0 {
					continue
				}
				if !inColumnBounds(dim, tx, ty) {
					continue
				}
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				heightBias := math.Abs(float64(dz)) * 0.8
				if dist+heightBias > float64(radius)+0.2 {
					continue
				}
				leaf := g.blockForPart(variant.leavesBlock, variant.name, "leaf", nil)
				placeIfAir(buffer, dim, tx, ty, tz, leaf)
			}
		}
	}
}

func (g *NoiseGenerator) buildCanopy(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, canopyStart, canopyTop int) {
	variant := placement.variant
	for z := canopyStart; z <= canopyTop; z++ {
		layerRadius := canopyRadiusForLevel(variant, z-canopyStart)
		for dx := -layerRadius; dx <= layerRadius; dx++ {
			for dy := -layerRadius; dy <= layerRadius; dy++ {
				tx := placement.localX + dx
				ty := placement.localY + dy
				if !inColumnBounds(dim, tx, ty) {
					continue
				}
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(layerRadius)+0.45 {
					continue
				}
				fade := float64(z-canopyStart) / float64(variant.canopyHeight)
				if dist+fade*1.5 > float64(layerRadius)+0.6 {
					continue
				}
				block := g.blockForPart(variant.leavesBlock, variant.name, "canopy", map[string]any{
					"layer": z - canopyStart,
				})
				placeIfAir(buffer, dim, tx, ty, z, block)
			}
		}

		accentRadius := layerRadius - 2
		if accentRadius < 2 {
			continue
		}
		for dx := -accentRadius; dx <= accentRadius; dx++ {
			for dy := -accentRadius; dy <= accentRadius; dy++ {
				if dx == 0 && dy == 0 {
					continue
				}
				tx := placement.localX + dx
				ty := placement.localY + dy
				if !inColumnBounds(dim, tx, ty) {
					continue
				}
				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(accentRadius)+0.25 {
					continue
				}
				if ((dx+dy)+(z-canopyStart))%3 != 0 {
					continue
				}
				accent := g.blockForPart(variant.accentBlock, variant.name, "canopyVeil", map[string]any{
					"layer": z - canopyStart,
				})
				placeIfAir(buffer, dim, tx, ty, z+1, accent)
			}
		}
	}
}

func (g *NoiseGenerator) decorateVeins(buffer *chunkWriteBuffer, dim world.Dimensions, placement treePlacement, baseLocalZ, trunkTop, canopyTop int) {
	variant := placement.variant
	if variant.veinBlock.Type == "" {
		return
	}
	directions := []struct{ dx, dy int }{{variant.radiusForLevel(0), 0}, {-variant.radiusForLevel(0), 0}, {0, variant.radiusForLevel(0)}, {0, -variant.radiusForLevel(0)}}
	for _, dir := range directions {
		for level := 0; level < variant.trunkHeight && baseLocalZ+level < dim.Height; level++ {
			tx := placement.localX + dir.dx
			ty := placement.localY + dir.dy
			tz := baseLocalZ + level
			if !inColumnBounds(dim, tx, ty) {
				continue
			}
			block := g.blockForPart(variant.veinBlock, variant.name, "trunkVein", map[string]any{
				"level": level,
			})
			setBlock(buffer, dim, tx, ty, tz, block)
		}
	}

	// Drape light through canopy center column.
	for z := trunkTop; z <= canopyTop && z < dim.Height; z++ {
		block := g.blockForPart(variant.veinBlock, variant.name, "canopyVein", map[string]any{
			"layer": z - trunkTop,
		})
		setBlock(buffer, dim, placement.localX, placement.localY, z, block)
	}
}

func (g *NoiseGenerator) blockForPart(proto world.Block, variantName, part string, extra map[string]any) world.Block {
	block := proto
	metadata := make(map[string]any, len(extra)+3)
	metadata["structure"] = "arboreal_complex"
	metadata["treeType"] = variantName
	metadata["part"] = part
	for k, v := range extra {
		metadata[k] = v
	}
	block.Metadata = metadata
	return block
}

func (variant *treeVariant) radiusForLevel(level int) int {
	if level < variant.stumpHeight {
		radius := variant.trunkRadius + variant.stumpRadius - level
		if radius < variant.trunkRadius {
			return variant.trunkRadius
		}
		return radius
	}
	return variant.trunkRadius
}

func canopyRadiusForLevel(variant *treeVariant, level int) int {
	if level < 0 {
		level = 0
	}
	if level > variant.canopyHeight {
		level = variant.canopyHeight
	}
	taper := float64(level) / float64(variant.canopyHeight+1)
	radius := float64(variant.canopyRadius) - taper*2.5
	if radius < float64(variant.trunkRadius+2) {
		radius = float64(variant.trunkRadius + 2)
	}
	return int(math.Ceil(radius))
}

func (g *NoiseGenerator) isForestCell(globalX, globalY int) bool {
	mask := forestMask(globalX, globalY, g.seed)
	return mask > 0.35
}

func (g *NoiseGenerator) selectTreeVariant(globalX, globalY int) *treeVariant {
	if len(g.treeVariants) == 0 {
		return nil
	}
	hash := hash3(globalX, globalY, int(g.seed^0xd1ce7))
	idx := int(hash % uint32(len(g.treeVariants)))
	if idx < 0 || idx >= len(g.treeVariants) {
		return &g.treeVariants[0]
	}
	return &g.treeVariants[idx]
}

func (g *NoiseGenerator) nearChunkEdge(localX, localY int, dim world.Dimensions, variant *treeVariant) bool {
	margin := variant.canopyRadius + variant.branchLength + 2
	if localX < margin || localY < margin {
		return true
	}
	if localX > dim.Width-margin-1 || localY > dim.Depth-margin-1 {
		return true
	}
	return false
}

func (g *NoiseGenerator) slopeTooSteep(buffer *chunkWriteBuffer, dim world.Dimensions, localX, localY, surfaceIdx int) bool {
	baseHeight := surfaceIdx
	directions := []struct{ dx, dy int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, dir := range directions {
		nx := localX + dir.dx
		ny := localY + dir.dy
		if !inColumnBounds(dim, nx, ny) {
			continue
		}
		column, ok := buffer.column(nx, ny)
		if !ok {
			continue
		}
		neighbor := columnSurfaceIndex(column)
		if neighbor < 0 {
			continue
		}
		if absInt(neighbor-baseHeight) > 4 {
			return true
		}
	}
	return false
}

func (g *NoiseGenerator) hasVerticalSpace(dim world.Dimensions, surfaceIdx int, variant *treeVariant) bool {
	required := surfaceIdx + variant.trunkHeight + variant.canopyHeight + 4
	return required < dim.Height
}

func (g *NoiseGenerator) checkForestSpacing(placements []treePlacement, variant *treeVariant, globalX, globalY int) bool {
	minSpacing := float64(variant.minSpacing)
	for _, placement := range placements {
		distance := math.Hypot(float64(globalX-placement.globalX), float64(globalY-placement.globalY))
		limit := math.Max(minSpacing, float64(placement.variant.minSpacing))
		if distance < limit {
			return false
		}
	}
	return true
}

func forestMask(globalX, globalY int, seed int64) float64 {
	base := random2D(globalX/8, globalY/8, seed)
	detail := random2D(globalX/3, globalY/3, seed^0x5f17)
	blend := (base*0.7 + detail*0.3 + 1) * 0.5
	if blend < 0 {
		blend = 0
	}
	if blend > 1 {
		blend = 1
	}
	return blend
}

func treeProbability(globalX, globalY int, seed int64) float64 {
	primary := random2D(globalX/5, globalY/5, seed^0x92b7)
	secondary := random2D(globalX/17, globalY/17, seed^0x12d4)
	blend := (primary*0.6 + secondary*0.4 + 1) * 0.5
	if blend < 0 {
		blend = 0
	}
	if blend > 1 {
		blend = 1
	}
	return blend
}

func columnSurfaceIndex(column []world.Block) int {
	for idx := len(column) - 1; idx >= 0; idx-- {
		if !blockIsEmpty(column[idx]) {
			return idx
		}
	}
	return -1
}

func isTopsoil(block world.Block) bool {
	if block.Metadata == nil {
		return false
	}
	if layer, ok := block.Metadata["layer"].(string); ok {
		return layer == "topsoil"
	}
	return false
}

func blockIsEmpty(block world.Block) bool {
	return block.Type == "" || block.Type == world.BlockAir
}

func withAppearance(block world.Block, material, color, texture string) world.Block {
	clone := block
	clone.Material = material
	clone.Color = color
	clone.Texture = texture
	return clone
}

func setBlock(buffer *chunkWriteBuffer, dim world.Dimensions, localX, localY, localZ int, block world.Block) {
	if !inColumnBounds(dim, localX, localY) || localZ < 0 {
		return
	}
	column, ok := buffer.column(localX, localY)
	if !ok {
		column = make([]world.Block, 0, localZ+1)
	}
	if localZ >= len(column) {
		expanded := make([]world.Block, localZ+1)
		copy(expanded, column)
		column = expanded
	}
	if blockIsEmpty(block) {
		column[localZ] = world.Block{}
	} else {
		column[localZ] = block
	}
	column = trimTrailingAir(column)
	buffer.setColumn(localX, localY, column)
}

func placeIfAir(buffer *chunkWriteBuffer, dim world.Dimensions, localX, localY, localZ int, block world.Block) {
	if !inColumnBounds(dim, localX, localY) || localZ < 0 {
		return
	}
	column, ok := buffer.column(localX, localY)
	if ok && localZ < len(column) && !blockIsEmpty(column[localZ]) {
		return
	}
	setBlock(buffer, dim, localX, localY, localZ, block)
}

func clearBlock(buffer *chunkWriteBuffer, dim world.Dimensions, localX, localY, localZ int) {
	if !inColumnBounds(dim, localX, localY) || localZ < 0 {
		return
	}
	setBlock(buffer, dim, localX, localY, localZ, world.Block{})
}

func inColumnBounds(dim world.Dimensions, localX, localY int) bool {
	return localX >= 0 && localY >= 0 && localX < dim.Width && localY < dim.Depth
}

func trimTrailingAir(column []world.Block) []world.Block {
	end := len(column)
	for end > 0 && blockIsEmpty(column[end-1]) {
		end--
	}
	return column[:end]
}

func orientationVector(orientation int) struct{ dx, dy int } {
	switch orientation % 4 {
	case 0:
		return struct{ dx, dy int }{dx: 1, dy: 0}
	case 1:
		return struct{ dx, dy int }{dx: 0, dy: 1}
	case 2:
		return struct{ dx, dy int }{dx: -1, dy: 0}
	case 3:
		return struct{ dx, dy int }{dx: 0, dy: -1}
	default:
		return struct{ dx, dy int }{dx: 1, dy: 0}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

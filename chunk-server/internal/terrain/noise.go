package terrain

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"chunkserver/internal/config"
	"chunkserver/internal/world"
)

// NoiseGenerator creates repeatable terrain using hashed value noise.
type NoiseGenerator struct {
	cfg                     config.TerrainConfig
	economy                 config.EconomyConfig
	seed                    int64
	randPool                sync.Pool
	topsoilSurfacePrototype world.Block
	topsoilPrototype        world.Block
	subsoilPrototype        world.Block
	stonePrototype          world.Block
	deepstonePrototype      world.Block
}

func NewNoiseGenerator(cfg config.TerrainConfig, economy config.EconomyConfig) *NoiseGenerator {
	generator := &NoiseGenerator{
		cfg:     cfg,
		economy: economy,
		seed:    cfg.Seed,
		randPool: sync.Pool{
			New: func() any {
				// Seed with time for uniqueness but override deterministically per use.
				return rand.New(rand.NewSource(time.Now().UnixNano()))
			},
		},
	}
	generator.initPrototypes()
	return generator
}

func (g *NoiseGenerator) initPrototypes() {
	topsoilSurface := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       90,
		MaxHitPoints:    90,
		ConnectingForce: 70,
		Weight:          6,
	}
	world.ApplyAppearance(&topsoilSurface, world.MaterialGrass)
	g.topsoilSurfacePrototype = topsoilSurface

	topsoil := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       90,
		MaxHitPoints:    90,
		ConnectingForce: 70,
		Weight:          6,
	}
	world.ApplyAppearance(&topsoil, world.MaterialDirt)
	g.topsoilPrototype = topsoil

	subsoil := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       130,
		MaxHitPoints:    130,
		ConnectingForce: 95,
		Weight:          9,
	}
	world.ApplyAppearance(&subsoil, world.MaterialDirt)
	g.subsoilPrototype = subsoil

	g.stonePrototype = world.Block{
		Type:            world.BlockSolid,
		HitPoints:       190,
		MaxHitPoints:    190,
		ConnectingForce: 150,
		Weight:          14,
	}

	g.deepstonePrototype = world.Block{
		Type:            world.BlockSolid,
		HitPoints:       240,
		MaxHitPoints:    240,
		ConnectingForce: 210,
		Weight:          18,
	}
}

func (g *NoiseGenerator) surfaceLevel(bounds world.Bounds, dim world.Dimensions) int {
	ratio := g.cfg.SurfaceRatio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.75
	}
	height := dim.Height - 1
	if height < 0 {
		height = 0
	}
	base := bounds.Min.Z + int(float64(height)*ratio)
	if base > bounds.Max.Z {
		return bounds.Max.Z
	}
	return base
}

func (g *NoiseGenerator) surfaceAmplitude(dim world.Dimensions) float64 {
	if g.cfg.AmplitudeRatio > 0 {
		return float64(dim.Height) * g.cfg.AmplitudeRatio
	}
	if g.cfg.Amplitude > 0 {
		return g.cfg.Amplitude
	}
	return float64(dim.Height) * 0.25
}

func (g *NoiseGenerator) undergroundLimit(bounds world.Bounds, dim world.Dimensions) int {
	ratio := g.cfg.UndergroundRatio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.6
	}
	limit := bounds.Min.Z + int(float64(dim.Height)*ratio)
	if limit > bounds.Max.Z {
		return bounds.Max.Z
	}
	return limit
}

func (g *NoiseGenerator) Generate(ctx context.Context, coord world.ChunkCoord, bounds world.Bounds, dim world.Dimensions) (*world.Chunk, error) {
	chunk := world.NewChunk(coord, bounds, dim)

	totalColumns := dim.Width * dim.Depth
	if totalColumns <= 0 {
		log.Printf("chunk %v generation progress: 100%%", coord)
		return chunk, nil
	}

	log.Printf("chunk %v generation progress: 0%%", coord)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	buffer := newChunkWriteBuffer(chunk, dim, 1<<28)

	surfaceBase := g.surfaceLevel(bounds, dim)
	amplitude := g.surfaceAmplitude(dim)
	undergroundCap := g.undergroundLimit(bounds, dim)

	type columnTask struct {
		localX int
		localY int
	}

	type columnResult struct {
		localX int
		localY int
		column []world.Block
		err    error
	}

	workers := g.workerCount(totalColumns)
	if workers <= 0 {
		workers = 1
	}

	tasks := make(chan columnTask, workers)
	results := make(chan columnResult, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				if err := ctx.Err(); err != nil {
					select {
					case results <- columnResult{err: err}:
					default:
					}
					return
				}

				globalX := bounds.Min.X + task.localX
				globalY := bounds.Min.Y + task.localY
				noise := g.fractalNoise(float64(globalX), float64(globalY))

				surfaceHeight := int(float64(surfaceBase) + noise*amplitude)
				surfaceHeight = clampInt(surfaceHeight, bounds.Min.Z, bounds.Max.Z)

				column := g.populateColumn(bounds, dim, task.localX, task.localY, surfaceHeight, noise, undergroundCap)

				select {
				case results <- columnResult{localX: task.localX, localY: task.localY, column: column}:
				case <-ctx.Done():
					if err := ctx.Err(); err != nil {
						select {
						case results <- columnResult{err: err}:
						default:
						}
					}
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		defer close(tasks)
		for x := 0; x < dim.Width; x++ {
			for y := 0; y < dim.Depth; y++ {
				select {
				case <-ctx.Done():
					return
				case tasks <- columnTask{localX: x, localY: y}:
				}
			}
		}
	}()

	generatedColumns := 0
	nextLogPercent := 10
	loggedComplete := false

	for result := range results {
		if result.err != nil {
			cancel()
			return nil, result.err
		}

		if err := buffer.Store(result.localX, result.localY, result.column); err != nil {
			cancel()
			return nil, err
		}

		generatedColumns++
		progress := generatedColumns * 100 / totalColumns
		if progress >= nextLogPercent {
			if progress > 100 {
				progress = 100
			}
			log.Printf("chunk %v generation progress: %d%%", coord, progress)
			if progress >= 100 {
				loggedComplete = true
				nextLogPercent = 110
			} else {
				nextLogPercent = ((progress / 10) + 1) * 10
			}
		}
	}

	if err := g.seedMineralVeins(buffer, bounds, dim); err != nil {
		return nil, err
	}

	if err := buffer.Flush(); err != nil {
		return nil, err
	}

	if !loggedComplete {
		log.Printf("chunk %v generation progress: 100%%", coord)
	}

	return chunk, nil
}

func (g *NoiseGenerator) populateColumn(bounds world.Bounds, dim world.Dimensions, localX, localY int, surfaceHeight int, noise float64, undergroundCap int) []world.Block {
	maxLocalZ := surfaceHeight - bounds.Min.Z
	if maxLocalZ >= dim.Height {
		maxLocalZ = dim.Height - 1
	}
	if maxLocalZ < 0 {
		return nil
	}

	globalX := bounds.Min.X + localX
	globalY := bounds.Min.Y + localY

	totalHeight := maxLocalZ + 1
	column := make([]world.Block, totalHeight)
	fillBlockRange(column, 0, totalHeight-1, g.deepstonePrototype)

	topsoilStart := maxLocalZ - 2
	if topsoilStart < 0 {
		topsoilStart = 0
	}
	subsoilStart := maxLocalZ - 12
	if subsoilStart < 0 {
		subsoilStart = 0
	}
	stoneStart := maxLocalZ - 64
	if stoneStart < 0 {
		stoneStart = 0
	}

	stoneEnd := subsoilStart - 1
	if stoneEnd >= stoneStart {
		if stoneEnd >= totalHeight {
			stoneEnd = totalHeight - 1
		}
		fillBlockRange(column, stoneStart, stoneEnd, g.stonePrototype)
	}

	subsoilEnd := topsoilStart - 1
	if subsoilEnd >= subsoilStart {
		if subsoilEnd >= totalHeight {
			subsoilEnd = totalHeight - 1
		}
		for idx := subsoilStart; idx <= subsoilEnd; idx++ {
			block := g.subsoilPrototype
			block.Metadata = map[string]any{"layer": "subsoil"}
			column[idx] = block
		}
	}

	for idx := topsoilStart; idx < totalHeight; idx++ {
		depth := maxLocalZ - idx
		var block world.Block
		if depth == 0 {
			block = g.topsoilSurfacePrototype
		} else {
			block = g.topsoilPrototype
		}
		block.Metadata = map[string]any{"layer": "topsoil"}
		column[idx] = block
	}

	globalZ := bounds.Min.Z
	for idx := 0; idx < totalHeight; idx++ {
		block := &column[idx]
		if globalZ < undergroundCap {
			block.ConnectingForce += 40
			block.Weight += 2
		}
		globalZ++
	}

	g.applyColumnInstability(column, maxLocalZ, globalX, globalY, noise)
	return column
}

func (g *NoiseGenerator) seedMineralVeins(buffer *chunkWriteBuffer, bounds world.Bounds, dim world.Dimensions) error {
	if buffer == nil {
		return fmt.Errorf("chunk write buffer is nil")
	}
	if len(buffer.columns) == 0 {
		buffer.recalculateUsage()
		return nil
	}

	for mineral, density := range g.economy.ResourceSpawnDensity {
		if density <= 0 {
			continue
		}

		for localX := 0; localX < dim.Width; localX++ {
			for localY := 0; localY < dim.Depth; localY++ {
				column, ok := buffer.column(localX, localY)
				if !ok || len(column) == 0 {
					continue
				}

				globalX := bounds.Min.X + localX
				globalY := bounds.Min.Y + localY
				hashVal := hash3(globalX, globalY, int(g.seed^int64(len(mineral))))
				chance := float64(hashVal&0xFFFF) / 0xFFFF
				if chance > density {
					continue
				}

				rng := g.random(hashVal)
				placements := veinSizeForDensity(density, rng)
				if placements > len(column) {
					placements = len(column)
				}
				g.scatterMinerals(buffer, column, localX, localY, mineral, placements, rng)
				g.releaseRandom(rng)
			}
		}
	}

	buffer.recalculateUsage()
	return nil
}

func (g *NoiseGenerator) scatterMinerals(buffer *chunkWriteBuffer, column []world.Block, localX, localY int, mineral string, placements int, rng *rand.Rand) {
	if placements <= 0 {
		return
	}
	used := make(map[int]struct{}, placements)
	placed := 0
	attempts := 0
	maxAttempts := placements * 6

	for placed < placements && attempts < maxAttempts {
		attempts++
		target := rng.Intn(len(column))
		if _, ok := used[target]; ok {
			continue
		}
		if !g.applyMineralToBlock(column, target, mineral) {
			continue
		}
		used[target] = struct{}{}
		placed++
	}

	if placed > 0 {
		buffer.setColumn(localX, localY, column)
	}
}

func veinSizeForDensity(density float64, rng *rand.Rand) int {
	base := 3 + int(math.Ceil(density*4))
	max := base + int(math.Ceil(density*6))
	if max < base {
		max = base
	}
	if base < 3 {
		base = 3
	}
	if max == base {
		return base
	}
	return base + rng.Intn(max-base+1)
}

func (g *NoiseGenerator) applyMineralToBlock(column []world.Block, localZ int, mineral string) bool {
	if localZ < 0 || localZ >= len(column) {
		return false
	}
	block := column[localZ]
	if block.Type == world.BlockAir {
		return false
	}
	if layer, ok := block.Metadata["layer"].(string); ok && layer == "topsoil" {
		return false
	}
	if block.ResourceYield == nil {
		block.ResourceYield = make(map[string]float64)
	}
	block.Type = world.BlockMineral
	block.ResourceYield[mineral] += 1
	if block.ConnectingForce < 130 {
		block.ConnectingForce = 130
	}
	if block.MaxHitPoints < 180 {
		block.MaxHitPoints = 180
	}
	if block.HitPoints < block.MaxHitPoints {
		block.HitPoints = block.MaxHitPoints
	}
	block.Weight += 3
	if block.Metadata == nil {
		block.Metadata = make(map[string]any)
	}
	block.Metadata["veinResource"] = mineral
	column[localZ] = block
	return true
}

func (g *NoiseGenerator) random(seed uint32) *rand.Rand {
	r := g.randPool.Get().(*rand.Rand)
	r.Seed(int64(seed)<<1 | 1)
	return r
}

func (g *NoiseGenerator) releaseRandom(r *rand.Rand) {
	if r == nil {
		return
	}
	g.randPool.Put(r)
}

type chunkWriteBuffer struct {
	chunk      *world.Chunk
	dim        world.Dimensions
	threshold  int64
	columns    map[int][]world.Block
	usageBytes int64
}

func newChunkWriteBuffer(chunk *world.Chunk, dim world.Dimensions, threshold int64) *chunkWriteBuffer {
	if threshold <= 0 {
		threshold = 1 << 30
	}
	return &chunkWriteBuffer{
		chunk:     chunk,
		dim:       dim,
		threshold: threshold,
		columns:   make(map[int][]world.Block),
	}
}

func (b *chunkWriteBuffer) Store(localX, localY int, column []world.Block) error {
	if b == nil {
		return fmt.Errorf("chunk write buffer is nil")
	}
	idx := b.index(localX, localY)
	b.columns[idx] = column
	b.usageBytes += columnMemory(column)
	if b.usageBytes >= b.threshold {
		return b.Flush()
	}
	return nil
}

func (b *chunkWriteBuffer) Flush() error {
	if b == nil || len(b.columns) == 0 {
		b.usageBytes = 0
		return nil
	}
	for idx, column := range b.columns {
		localX := idx % b.dim.Width
		localY := idx / b.dim.Width
		if ok := b.chunk.SetColumnBlocks(localX, localY, column); !ok {
			return fmt.Errorf("chunk %v failed to persist column (%d,%d)", b.chunk.Key, localX, localY)
		}
	}
	b.columns = make(map[int][]world.Block)
	b.usageBytes = 0
	return nil
}

func (b *chunkWriteBuffer) index(localX, localY int) int {
	return localY*b.dim.Width + localX
}

func (b *chunkWriteBuffer) column(localX, localY int) ([]world.Block, bool) {
	if b == nil {
		return nil, false
	}
	column, ok := b.columns[b.index(localX, localY)]
	return column, ok
}

func (b *chunkWriteBuffer) setColumn(localX, localY int, column []world.Block) {
	if b == nil {
		return
	}
	b.columns[b.index(localX, localY)] = column
}

func (b *chunkWriteBuffer) recalculateUsage() {
	if b == nil {
		return
	}
	var total int64
	for _, column := range b.columns {
		total += columnMemory(column)
	}
	b.usageBytes = total
}

func columnMemory(column []world.Block) int64 {
	if len(column) == 0 {
		return 0
	}
	blockSize := int64(unsafe.Sizeof(world.Block{}))
	return int64(len(column)) * blockSize
}

func fillBlockRange(column []world.Block, start, end int, value world.Block) {
	if len(column) == 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	if end >= len(column) {
		end = len(column) - 1
	}
	if start > end {
		return
	}
	column[start] = value
	filled := 1
	remaining := end - start + 1
	for filled < remaining {
		copyLen := filled
		if copyLen > remaining-filled {
			copyLen = remaining - filled
		}
		copy(column[start+filled:], column[start:start+copyLen])
		filled += copyLen
	}
}

func (g *NoiseGenerator) applyColumnInstability(column []world.Block, maxLocalZ int, globalX, globalY int, noise float64) {
	rangeSize := maxLocalZ - 5
	if rangeSize <= 0 {
		return
	}

	noiseBias := (noise + 1) * 0.5
	threshold := 0.05 + 0.15*noiseBias
	expected := int(math.Round(float64(rangeSize) * threshold))
	if expected <= 0 {
		return
	}
	if expected > rangeSize {
		expected = rangeSize
	}

	rng := newDeterministicRNG(globalX, globalY, g.seed)
	selected := make(map[int]struct{}, expected)
	attempts := 0
	maxAttempts := rangeSize * 4

	for len(selected) < expected && attempts < maxAttempts {
		offset := rng.nextInt(rangeSize)
		attempts++
		if _, ok := selected[offset]; ok {
			continue
		}
		depth := 6 + offset
		idx := maxLocalZ - depth
		if idx < 0 || idx >= len(column) {
			continue
		}
		block := &column[idx]
		if block.Type == world.BlockAir {
			continue
		}
		selected[offset] = struct{}{}

		block.Type = world.BlockUnstable
		block.ConnectingForce *= 0.45
		block.HitPoints *= 0.8
		block.MaxHitPoints = block.HitPoints
		block.Weight *= 0.92
		if block.Metadata == nil {
			block.Metadata = make(map[string]any, 2)
		}
		penalty := threshold * (0.5 + 0.5*(float64(rng.next()&0xFFFF)/0xFFFF))
		block.Metadata["unstable"] = true
		block.Metadata["stabilityPenalty"] = penalty
	}
}

type deterministicRNG struct {
	state uint64
}

func newDeterministicRNG(x, y int, seed int64) *deterministicRNG {
	state := uint64(uint32(x))<<32 ^ uint64(uint32(y))<<1 ^ uint64(seed)
	if state == 0 {
		state = 0x9e3779b97f4a7c15
	}
	return &deterministicRNG{state: state}
}

func (r *deterministicRNG) next() uint64 {
	r.state ^= r.state << 7
	r.state ^= r.state >> 9
	r.state ^= r.state << 8
	return r.state
}

func (r *deterministicRNG) nextInt(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func (g *NoiseGenerator) fractalNoise(x, y float64) float64 {
	frequency := g.cfg.Frequency
	amplitude := 1.0
	noiseSum := 0.0
	maxAmplitude := 0.0

	for i := 0; i < g.cfg.Octaves; i++ {
		noise := g.valueNoise(x*frequency, y*frequency)
		noiseSum += noise * amplitude
		maxAmplitude += amplitude
		amplitude *= g.cfg.Persistence
		frequency *= g.cfg.Lacunarity
	}

	if maxAmplitude == 0 {
		return 0
	}
	return noiseSum / maxAmplitude
}

func (g *NoiseGenerator) valueNoise(x, y float64) float64 {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1

	sx := smooth(x - float64(x0))
	sy := smooth(y - float64(y0))

	n0 := random2D(x0, y0, g.seed)
	n1 := random2D(x1, y0, g.seed)
	ix0 := lerp(n0, n1, sx)

	n2 := random2D(x0, y1, g.seed)
	n3 := random2D(x1, y1, g.seed)
	ix1 := lerp(n2, n3, sx)

	return lerp(ix0, ix1, sy)
}

func smooth(t float64) float64 {
	return t * t * (3 - 2*t)
}

func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

func random2D(x, y int, seed int64) float64 {
	return float64(hash3(x, y, int(seed))&0xFFFF)/0x8000 - 1.0
}

func hash3(x, y, z int) uint32 {
	h := uint32(x*374761393 + y*668265263 + z*2147483647)
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (g *NoiseGenerator) workerCount(totalColumns int) int {
	if totalColumns <= 0 {
		return 0
	}

	if g.cfg.Workers > 0 {
		if g.cfg.Workers < totalColumns {
			return g.cfg.Workers
		}
		return totalColumns
	}

	workers := runtime.GOMAXPROCS(0) * 2
	if workers <= 0 {
		workers = 1
	}
	if workers > totalColumns {
		workers = totalColumns
	}
	if workers <= 0 {
		workers = 1
	}
	return workers
}

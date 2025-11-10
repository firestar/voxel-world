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
	cfg            config.TerrainConfig
	economy        config.EconomyConfig
	seed           int64
	surface        int
	undergroundCap int
	randPool       sync.Pool
}

func NewNoiseGenerator(cfg config.TerrainConfig, economy config.EconomyConfig) *NoiseGenerator {
	return &NoiseGenerator{
		cfg:            cfg,
		economy:        economy,
		seed:           cfg.Seed,
		surface:        768,
		undergroundCap: 640,
		randPool: sync.Pool{
			New: func() any {
				// Seed with time for uniqueness but override deterministically per use.
				return rand.New(rand.NewSource(time.Now().UnixNano()))
			},
		},
	}
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

	workers := runtime.GOMAXPROCS(0) * 2
	if workers <= 0 {
		workers = 1
	}

	tasks := make(chan columnTask)
	results := make(chan columnResult)

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

				surfaceHeight := g.computeSurfaceHeight(noise)
				surfaceHeight = clampInt(surfaceHeight, bounds.Min.Z, bounds.Max.Z)

				column := g.populateColumn(bounds, dim, task.localX, task.localY, surfaceHeight, noise)

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

func (g *NoiseGenerator) populateColumn(bounds world.Bounds, dim world.Dimensions, localX, localY int, surfaceHeight int, noise float64) []world.Block {
	maxLocalZ := surfaceHeight - bounds.Min.Z
	if maxLocalZ >= dim.Height {
		maxLocalZ = dim.Height - 1
	}
	if maxLocalZ < 0 {
		return nil
	}

	globalX := bounds.Min.X + localX
	globalY := bounds.Min.Y + localY

	column := make([]world.Block, maxLocalZ+1)
	for localZ := 0; localZ <= maxLocalZ; localZ++ {
		globalZ := bounds.Min.Z + localZ
		column[localZ] = g.composeTerrainBlock(globalX, globalY, globalZ, surfaceHeight, noise)
	}
	return column
}

func (g *NoiseGenerator) composeTerrainBlock(globalX, globalY, globalZ int, surfaceHeight int, noise float64) world.Block {
	depth := surfaceHeight - globalZ
	if depth < 0 {
		depth = 0
	}

	block := world.Block{
		Type:            world.BlockSolid,
		HitPoints:       160,
		MaxHitPoints:    160,
		ConnectingForce: 120,
		Weight:          12,
		Metadata:        make(map[string]any),
	}

	switch {
	case depth <= 2:
		block.HitPoints = 90
		block.MaxHitPoints = 90
		block.ConnectingForce = 70
		block.Weight = 6
		block.Metadata["layer"] = "topsoil"
		if depth == 0 {
			world.ApplyAppearance(&block, world.MaterialGrass)
		} else {
			world.ApplyAppearance(&block, world.MaterialDirt)
		}
	case depth <= 12:
		block.HitPoints = 130
		block.MaxHitPoints = 130
		block.ConnectingForce = 95
		block.Weight = 9
		block.Metadata["layer"] = "subsoil"
		world.ApplyAppearance(&block, world.MaterialDirt)
	case depth <= 64:
		block.HitPoints = 190
		block.MaxHitPoints = 190
		block.ConnectingForce = 150
		block.Weight = 14
		block.Metadata["layer"] = "stone"
	default:
		block.HitPoints = 240
		block.MaxHitPoints = 240
		block.ConnectingForce = 210
		block.Weight = 18
		block.Metadata["layer"] = "deepstone"
	}

	if globalZ < g.undergroundCap/4 {
		block.ConnectingForce += 40
		block.Weight += 2
	}

	g.applyUnstableVariation(&block, globalX, globalY, globalZ, depth, noise)
	return block
}

func (g *NoiseGenerator) applyUnstableVariation(block *world.Block, globalX, globalY, globalZ, depth int, noise float64) {
	if depth < 6 {
		return
	}

	hashVal := hash3(globalX+depth, globalY-depth, int(g.seed)+globalZ)
	probability := float64(hashVal&0xFFFF) / 0xFFFF
	noiseBias := (noise + 1) * 0.5
	threshold := 0.05 + 0.15*noiseBias

	if probability > threshold {
		return
	}

	block.Type = world.BlockUnstable
	block.ConnectingForce *= 0.45
	block.HitPoints *= 0.8
	block.MaxHitPoints = block.HitPoints
	block.Weight *= 0.92
	block.Metadata["unstable"] = true
	block.Metadata["stabilityPenalty"] = probability
}

func (g *NoiseGenerator) computeSurfaceHeight(noise float64) int {
	return int(float64(g.surface) + noise*g.cfg.Amplitude)
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

		visited := make(map[veinCoord]struct{})
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
				startZ := rng.Intn(len(column))
				start := veinCoord{X: localX, Y: localY, Z: startZ}
				if _, already := visited[start]; already {
					g.releaseRandom(rng)
					continue
				}

				placed := g.growMineralVein(buffer, dim, mineral, density, rng, start, visited)
				g.releaseRandom(rng)
				if placed == 0 {
					continue
				}
			}
		}
	}

	buffer.recalculateUsage()
	return nil
}

func (g *NoiseGenerator) growMineralVein(buffer *chunkWriteBuffer, dim world.Dimensions, mineral string, density float64, rng *rand.Rand, start veinCoord, visited map[veinCoord]struct{}) int {
	queue := []veinCoord{start}
	placed := 0
	target := veinSizeForDensity(density, rng)
	diagonalPlaced := false

	for len(queue) > 0 && placed < target {
		current := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}

		if current.X < 0 || current.Y < 0 || current.Z < 0 ||
			current.X >= dim.Width || current.Y >= dim.Depth || current.Z >= dim.Height {
			continue
		}

		column, ok := buffer.column(current.X, current.Y)
		if !ok || current.Z >= len(column) {
			continue
		}

		if !g.applyMineralToBlock(column, current.Z, mineral) {
			continue
		}

		buffer.setColumn(current.X, current.Y, column)
		placed++

		g.enqueueHorizontalNeighbor(&queue, current, dim, rng)
		g.enqueueVerticalNeighbors(&queue, current, dim, rng)
		g.enqueueDiagonalNeighbors(&queue, current, dim, rng)

		if !diagonalPlaced && placed >= 2 {
			if g.forceDiagonalNeighbor(&queue, current, dim, rng) {
				diagonalPlaced = true
			}
		}
	}

	return placed
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

func (g *NoiseGenerator) enqueueHorizontalNeighbor(queue *[]veinCoord, current veinCoord, dim world.Dimensions, rng *rand.Rand) {
	horizontalOffsets := [...]veinCoord{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}}
	chosen := horizontalOffsets[rng.Intn(len(horizontalOffsets))]
	g.tryEnqueue(queue, current, chosen, dim)
	for _, off := range horizontalOffsets {
		if rng.Float64() < 0.55 {
			g.tryEnqueue(queue, current, off, dim)
		}
	}
}

func (g *NoiseGenerator) enqueueVerticalNeighbors(queue *[]veinCoord, current veinCoord, dim world.Dimensions, rng *rand.Rand) {
	verticalOffsets := [...]veinCoord{{0, 0, 1}, {0, 0, -1}}
	for _, off := range verticalOffsets {
		if rng.Float64() < 0.65 {
			g.tryEnqueue(queue, current, off, dim)
		}
	}
}

func (g *NoiseGenerator) enqueueDiagonalNeighbors(queue *[]veinCoord, current veinCoord, dim world.Dimensions, rng *rand.Rand) {
	diagonalOffsets := [...]veinCoord{
		{1, 1, 0}, {1, -1, 0}, {-1, 1, 0}, {-1, -1, 0},
		{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1},
		{1, 0, -1}, {-1, 0, -1}, {0, 1, -1}, {0, -1, -1},
		{1, 1, 1}, {1, -1, 1}, {-1, 1, 1}, {-1, -1, 1},
		{1, 1, -1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, -1},
	}
	for _, off := range diagonalOffsets {
		if rng.Float64() < 0.4 {
			g.tryEnqueue(queue, current, off, dim)
		}
	}
}

func (g *NoiseGenerator) forceDiagonalNeighbor(queue *[]veinCoord, current veinCoord, dim world.Dimensions, rng *rand.Rand) bool {
	candidates := []veinCoord{{1, 1, 0}, {1, -1, 0}, {-1, 1, 0}, {-1, -1, 0}, {1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1}, {1, 0, -1}, {-1, 0, -1}, {0, 1, -1}, {0, -1, -1}}
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	for _, off := range candidates {
		if g.tryEnqueue(queue, current, off, dim) {
			return true
		}
	}
	return false
}

func (g *NoiseGenerator) tryEnqueue(queue *[]veinCoord, current veinCoord, offset veinCoord, dim world.Dimensions) bool {
	next := veinCoord{X: current.X + offset.X, Y: current.Y + offset.Y, Z: current.Z + offset.Z}
	if next.X < 0 || next.Y < 0 || next.Z < 0 ||
		next.X >= dim.Width || next.Y >= dim.Depth || next.Z >= dim.Height {
		return false
	}
	*queue = append(*queue, next)
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

type veinCoord struct {
	X int
	Y int
	Z int
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

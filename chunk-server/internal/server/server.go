package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"chunkserver/internal/ai"
	"chunkserver/internal/config"
	"chunkserver/internal/entities"
	"chunkserver/internal/migration"
	"chunkserver/internal/network"
	"chunkserver/internal/pathfinding"
	"chunkserver/internal/terrain"
	"chunkserver/internal/world"
)

type Server struct {
	cfg       *config.Config
	world     *world.Manager
	entities  *entities.Manager
	navigator *pathfinding.BlockNavigator
	net       *network.Server
	logger    *log.Logger

	movementWorkers int

	ai *ai.Coordinator

	chunkCursor       world.LocalChunkIndex
	streamSeq         uint64
	dirtyEntities     map[entities.ID]entities.Entity
	dirtyChunks       map[world.ChunkCoord]struct{}
	dirtyChunkQueue   []world.ChunkCoord
	deltaBuffer       *deltaAccumulator
	deltaSeq          uint64
	neighbors         *neighborManager
	neighborSeq       uint64
	migrationQueue    *migration.Queue
	inFlightTransfers map[entities.ID]migration.Request
	transferSeq       uint64

	dirtyMu sync.Mutex
}

const (
	collapseImpactRadius = 3.5
	collapseImpactDamage = 45.0
)

func New(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	logger := log.New(log.Writer(), "chunk-server ", log.LstdFlags|log.Lmicroseconds)
	netSrv, err := network.Listen(cfg.Network.ListenUDP, logger, cfg.Network.MaxDatagramSizeBytes)
	if err != nil {
		return nil, err
	}

	region := world.NewServerRegion(cfg)
	terrainGen := terrain.NewNoiseGenerator(cfg.Terrain, cfg.Economy)
	worldManager := world.NewManager(region, terrainGen)

	entityManager := entities.NewManager(cfg.Server.ID)
	navigator := pathfinding.NewBlockNavigator(region, worldManager)

	workers := cfg.Entities.MovementWorkers
	if workers <= 0 {
		workers = 1
	}

	srv := &Server{
		cfg:               cfg,
		world:             worldManager,
		entities:          entityManager,
		navigator:         navigator,
		net:               netSrv,
		logger:            logger,
		movementWorkers:   workers,
		dirtyEntities:     make(map[entities.ID]entities.Entity),
		dirtyChunks:       make(map[world.ChunkCoord]struct{}),
		deltaBuffer:       newDeltaAccumulator(),
		neighbors:         newNeighborManager(region, cfg.Network.NeighborEndpoints),
		migrationQueue:    migration.NewQueue(),
		inFlightTransfers: make(map[entities.ID]migration.Request),
	}
	var lookup ai.NeighborLookup
	if srv.neighbors != nil {
		lookup = func(chunk world.ChunkCoord) (ai.NeighborOwnership, bool) {
			info, ok := srv.neighbors.ownership(chunk)
			if !ok {
				return ai.NeighborOwnership{}, false
			}
			return ai.NeighborOwnership{
				ServerID:     info.serverID,
				Endpoint:     info.endpoint,
				RegionOrigin: info.origin,
				RegionSize:   info.size,
			}, true
		}
	}
	srv.ai = ai.NewCoordinator(region, entityManager, navigator, lookup)
	srv.registerHandlers()
	return srv, nil
}

func (s *Server) registerHandlers() {
	s.net.Register(network.MessageNeighborHello, s.onNeighborHello)
	s.net.Register(network.MessageNeighborAck, s.onNeighborAck)
	s.net.Register(network.MessageEntityQuery, s.onEntityQuery)
	s.net.Register(network.MessagePathRequest, s.onPathRequest)
	s.net.Register(network.MessageTransferClaim, s.onTransferClaim)
	s.net.Register(network.MessageTransferRequest, s.onTransferRequest)
	s.net.Register(network.MessageTransferAck, s.onTransferAck)
}

func (s *Server) Run(ctx context.Context) error {
	defer s.net.Close()

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		if err := s.net.Serve(ctx); err != nil && ctx.Err() == nil {
			s.logger.Printf("network server stopped: %v", err)
			cancel()
		}
	}()

	s.announceToMainServers()

	movement := newMovementEngine(s, s.cfg.Server.TickRate, s.movementWorkers)
	movement.Start(ctx)
	defer func() {
		cancel()
		movement.Wait()
	}()

	stateTicker := time.NewTicker(s.cfg.Server.StateStreamRate)
	defer stateTicker.Stop()

	entityTicker := time.NewTicker(s.cfg.Entities.EntityTickRate)
	defer entityTicker.Stop()

	var discoveryTicker *time.Ticker
	var discoveryC <-chan time.Time
	if interval := s.cfg.Network.DiscoveryInterval; interval > 0 {
		discoveryTicker = time.NewTicker(interval)
		discoveryC = discoveryTicker.C
		defer discoveryTicker.Stop()
	}

	if discoveryC != nil {
		s.discoverNeighbors(time.Now())
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-entityTicker.C:
			s.flushDirtyEntities()
			s.flushVoxelDeltas()
			s.processMigrationQueue()
		case <-stateTicker.C:
			s.broadcastChunkSummaries(ctx)
		case <-discoveryC:
			s.discoverNeighbors(time.Now())
		}
	}
}

func (s *Server) tickEntities(delta time.Duration, workers int) {
	if s.ai != nil {
		s.ai.Tick(delta)
	}
	physics := entities.PhysicsParams{
		Gravity:         9.8,
		AirDrag:         0.4,
		GroundFriction:  4,
		MaxFallSpeed:    150,
		SupportsGravity: true,
	}

	dirty := s.entities.ApplyConcurrent(workers, func(ent *entities.Entity) {
		switch ent.Kind {
		case entities.KindProjectile:
			s.tickProjectile(ent, delta, physics)
		default:
			s.tickUnit(ent, delta, physics)
		}
	})

	s.recordDirtyEntities(dirty)
}

func (s *Server) tickProjectile(ent *entities.Entity, delta time.Duration, physics entities.PhysicsParams) {
	ent.ApplyGravity(physics, delta)
	ent.ApplyDrag(physics, delta)
	ent.Advance(delta)
	if life, ok := ent.ReduceAttribute("projectile_life", delta.Seconds()); ok && life <= 0 {
		s.handleProjectileImpact(ent)
		ent.FlagCollapse()
		return
	}
	s.updateEntityChunk(ent)
	if pos := ent.PositionVec(); pos.Z <= 0 {
		s.handleProjectileImpact(ent)
		ent.FlagCollapse()
		return
	}
}

func (s *Server) tickUnit(ent *entities.Entity, delta time.Duration, physics entities.PhysicsParams) {
	if value, ok := ent.Attribute("migration_pending"); ok && value > 0 {
		return
	}
	if !ent.Capabilities.CanFly {
		ent.ApplyGravity(physics, delta)
		ent.ApplyGroundFriction(physics.GroundFriction, delta)
	} else {
		ent.ApplyDrag(physics, delta)
	}
	ent.Advance(delta)
	ent.ClampZ(0)
	s.updateEntityChunk(ent)
}

func (s *Server) handleProjectileImpact(ent *entities.Entity) {
	if flagged, ok := ent.Attribute("_detonated"); ok && flagged > 0 {
		return
	}
	ent.SetAttribute("_detonated", 1)

	pos := ent.PositionVec()
	center := world.BlockCoord{
		X: int(math.Floor(pos.X)),
		Y: int(math.Floor(pos.Y)),
		Z: int(math.Floor(pos.Z)),
	}
	if center.Z < 0 {
		center.Z = 0
	}

	radius := 3.0
	if r, ok := ent.Attribute("explosion_radius"); ok && r > 0 {
		radius = r
	}
	damage := 250.0
	if d, ok := ent.Attribute("explosion_damage"); ok && d > 0 {
		damage = d
	}

	summary, err := s.world.ApplyExplosion(context.Background(), center, radius, damage)
	if err != nil {
		s.logger.Printf("apply explosion at %v: %v", center, err)
		return
	}
	s.queueVoxelDeltas(summary)
	s.damageEntitiesFromCollapses(summary)
	s.markChunksDirty(summary.DirtyChunks())

	if changes := summary.Changes(); len(changes) > 0 {
		s.logger.Printf("projectile %s detonated at %v affecting %d blocks", ent.ID, center, len(changes))
	}
}

func (s *Server) updateEntityChunk(ent *entities.Entity) {
	region := s.world.Region()
	pos := ent.PositionVec()
	blockX := int(math.Floor(pos.X))
	blockY := int(math.Floor(pos.Y))
	chunkCoord := world.ChunkCoord{
		X: floorDiv(blockX, region.ChunkDimension.Width),
		Y: floorDiv(blockY, region.ChunkDimension.Depth),
	}
	if region.ContainsGlobalChunk(chunkCoord) {
		if chunkCoord != ent.Chunk.Chunk {
			s.entities.Transfer(ent.ID, chunkCoord, s.cfg.Server.ID)
			s.recordDirtyEntity(ent)
			s.prefetchChunkNeighborhood(chunkCoord)
		}
		return
	}
	s.queueMigration(ent, chunkCoord)
}

func (s *Server) evaluateMigration(ent *entities.Entity) {
	// Intentionally left for future heuristic-based prefetching.
	_ = ent
}

func (s *Server) queueMigration(ent *entities.Entity, targetChunk world.ChunkCoord) {
	if s.migrationQueue == nil || s.neighbors == nil {
		return
	}
	if value, ok := ent.Attribute("migration_pending"); ok && value > 0 {
		return
	}
	info, ok := s.neighbors.neighborForChunk(targetChunk)
	if !ok {
		s.logger.Printf("migration: no neighbor found for chunk %v (entity %s)", targetChunk, ent.ID)
		return
	}
	endpoint := info.endpoint()
	if endpoint == "" {
		s.logger.Printf("migration: neighbor %s missing endpoint for entity %s", info.serverID, ent.ID)
		return
	}

	ent.SetAttribute("migration_pending", 1)
	req := migration.Request{
		EntityID:       ent.ID,
		EntitySnapshot: ent.Snapshot(),
		TargetChunk:    targetChunk,
		TargetServer:   info.serverID,
		TargetEndpoint: endpoint,
		QueuedAt:       time.Now(),
		Reason:         "boundary_exit",
	}
	s.migrationQueue.Enqueue(req)
	s.recordDirtyEntity(ent)
}

func (s *Server) processMigrationQueue() {
	if s.migrationQueue == nil {
		return
	}
	batch := s.migrationQueue.Drain(8)
	for _, req := range batch {
		if _, exists := s.inFlightTransfers[req.EntityID]; exists {
			continue
		}
		if ent, ok := s.entities.Entity(req.EntityID); ok {
			req.EntitySnapshot = ent.Snapshot()
		} else {
			continue
		}
		if err := s.sendMigrationRequest(req); err != nil {
			s.logger.Printf("migration: send request for entity %s failed: %v", req.EntityID, err)
			s.migrationQueue.Enqueue(req)
		}
	}
}

func (s *Server) sendMigrationRequest(req migration.Request) error {
	if req.TargetEndpoint == "" {
		return fmt.Errorf("missing target endpoint")
	}
	state := serializeEntity(req.EntitySnapshot)
	if state.Attributes == nil {
		state.Attributes = make(map[string]float64)
	}
	state.Attributes["migration_pending"] = 1
	nonce := s.nextTransferNonce()
	msg := network.TransferRequest{
		EntityID:     string(req.EntityID),
		FromServer:   s.cfg.Server.ID,
		ToServer:     req.TargetServer,
		GlobalChunkX: req.TargetChunk.X,
		GlobalChunkY: req.TargetChunk.Y,
		Reason:       req.Reason,
		State:        state,
		Nonce:        nonce,
		Timestamp:    time.Now().UTC(),
	}
	if err := s.net.Send(req.TargetEndpoint, network.MessageTransferRequest, msg); err != nil {
		return err
	}
	req.Nonce = nonce
	s.inFlightTransfers[req.EntityID] = req
	return nil
}

func (s *Server) discoverNeighbors(now time.Time) {
	if s.neighbors == nil {
		return
	}
	targets := s.neighbors.discoveryTargets(now, s.cfg.Network.DiscoveryInterval)
	if len(targets) == 0 {
		return
	}

	region := s.world.Region()
	nowUTC := now.UTC()
	for _, target := range targets {
		if target.Endpoint == "" {
			continue
		}
		nonce := s.nextNeighborNonce()
		hello := network.NeighborHello{
			ServerID:      s.cfg.Server.ID,
			Listen:        s.cfg.Network.ListenUDP,
			RegionOriginX: region.Origin.X,
			RegionOriginY: region.Origin.Y,
			RegionSize:    region.ChunksPerAxis,
			DeltaX:        target.Delta.X,
			DeltaY:        target.Delta.Y,
			Timestamp:     nowUTC,
			Nonce:         nonce,
		}
		if err := s.net.Send(target.Endpoint, network.MessageNeighborHello, hello); err != nil {
			s.logger.Printf("neighbor hello to %s failed: %v", target.Endpoint, err)
			continue
		}
		s.neighbors.markHelloSent(target.Delta, target.Endpoint, nonce, now)
		s.logger.Printf("neighbor hello sent to %s (delta %d,%d)", target.Endpoint, target.Delta.X, target.Delta.Y)
	}
}

func (s *Server) nextNeighborNonce() uint64 {
	s.neighborSeq++
	return s.neighborSeq
}

func (s *Server) recordDirtyEntities(list []entities.Entity) {
	if len(list) == 0 {
		return
	}
	s.dirtyMu.Lock()
	if s.dirtyEntities == nil {
		s.dirtyEntities = make(map[entities.ID]entities.Entity)
	}
	for _, ent := range list {
		s.dirtyEntities[ent.ID] = ent
	}
	s.dirtyMu.Unlock()
}

func (s *Server) recordDirtyEntity(ent *entities.Entity) {
	s.dirtyMu.Lock()
	if s.dirtyEntities == nil {
		s.dirtyEntities = make(map[entities.ID]entities.Entity)
	}
	snapshot := ent.Snapshot()
	s.dirtyEntities[ent.ID] = snapshot
	ent.MarkClean()
	s.dirtyMu.Unlock()
}

func (s *Server) damageEntitiesFromCollapses(summary *world.DamageSummary) {
	collapsed := summary.CollapsedBlocks()
	if len(collapsed) == 0 {
		return
	}

	region := s.world.Region()
	perChunk := make(map[world.ChunkCoord][]world.BlockCoord)
	for _, coord := range collapsed {
		chunkCoord, ok := region.LocateBlock(coord)
		if !ok {
			continue
		}
		perChunk[chunkCoord] = append(perChunk[chunkCoord], coord)
	}

	for chunkCoord, coords := range perChunk {
		entities := s.entities.MutableByChunk(chunkCoord)
		if len(entities) == 0 {
			continue
		}
		for _, ent := range entities {
			pos := ent.PositionVec()
			for _, block := range coords {
				dx := pos.X - float64(block.X)
				dy := pos.Y - float64(block.Y)
				dz := pos.Z - float64(block.Z)
				distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if distance > collapseImpactRadius {
					continue
				}
				damage := collapseImpactDamage * (1 - distance/collapseImpactRadius)
				if damage <= 0 {
					continue
				}
				ent.ApplyDamage(damage)
				s.recordDirtyEntity(ent)
				break
			}
		}
	}
}

func (s *Server) markChunksDirty(chunks []world.ChunkCoord) {
	if len(chunks) == 0 {
		return
	}
	for _, coord := range chunks {
		if _, exists := s.dirtyChunks[coord]; exists {
			continue
		}
		s.dirtyChunks[coord] = struct{}{}
		s.dirtyChunkQueue = append(s.dirtyChunkQueue, coord)
	}
}

func (s *Server) popDirtyChunk() (world.ChunkCoord, bool) {
	for len(s.dirtyChunkQueue) > 0 {
		coord := s.dirtyChunkQueue[0]
		s.dirtyChunkQueue = s.dirtyChunkQueue[1:]
		if _, ok := s.dirtyChunks[coord]; !ok {
			continue
		}
		delete(s.dirtyChunks, coord)
		return coord, true
	}
	return world.ChunkCoord{}, false
}

func (s *Server) prefetchChunkNeighborhood(center world.ChunkCoord) {
	region := s.world.Region()
	if !region.ContainsGlobalChunk(center) {
		return
	}

	neighbors := make([]world.ChunkCoord, 0, 9)
	neighbors = append(neighbors, center)
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			neighbor := world.ChunkCoord{X: center.X + dx, Y: center.Y + dy}
			if !region.ContainsGlobalChunk(neighbor) {
				continue
			}
			neighbors = append(neighbors, neighbor)
		}
	}

	s.markChunksDirty(neighbors)
}

func (s *Server) queueVoxelDeltas(summary *world.DamageSummary) {
	if summary == nil {
		return
	}

	changes := summary.Changes()
	if len(changes) == 0 {
		return
	}

	region := s.world.Region()
	for _, change := range changes {
		chunkCoord, ok := region.LocateBlock(change.Coord)
		if !ok {
			continue
		}
		if s.deltaBuffer == nil {
			s.deltaBuffer = newDeltaAccumulator()
		}
		s.deltaBuffer.add(chunkCoord, change)
	}
}

func (s *Server) flushVoxelDeltas() {
	if s.deltaBuffer == nil {
		return
	}
	deltas := s.deltaBuffer.flush(s.cfg.Server.ID, &s.deltaSeq)
	if len(deltas) == 0 {
		return
	}

	for _, delta := range deltas {
		for _, endpoint := range s.cfg.Network.MainServerEndpoints {
			if err := s.net.Send(endpoint, network.MessageChunkDelta, delta); err != nil {
				s.logger.Printf("chunk delta send to %s: %v", endpoint, err)
			}
		}
	}
}

func (s *Server) flushDirtyEntities() {
	s.dirtyMu.Lock()
	size := len(s.dirtyEntities)
	if size == 0 {
		s.dirtyMu.Unlock()
		return
	}

	list := make([]entities.Entity, 0, size)
	for _, ent := range s.dirtyEntities {
		list = append(list, ent)
	}

	s.dirtyEntities = make(map[entities.ID]entities.Entity, size)
	s.dirtyMu.Unlock()
	s.streamEntities(list)
}

func (s *Server) streamEntities(list []entities.Entity) {
	if len(list) == 0 {
		return
	}
	batch := network.EntityBatch{
		ServerID:  s.cfg.Server.ID,
		Seq:       s.streamSeq,
		Timestamp: time.Now().UTC(),
		Entities:  make([]network.EntityState, 0, len(list)),
	}
	s.streamSeq++

	for _, ent := range list {
		batch.Entities = append(batch.Entities, serializeEntity(ent))
	}

	for _, endpoint := range s.cfg.Network.MainServerEndpoints {
		if err := s.net.Send(endpoint, network.MessageEntityUpdate, batch); err != nil {
			s.logger.Printf("entity batch send to %s: %v", endpoint, err)
		}
	}
}

func (s *Server) broadcastChunkSummaries(ctx context.Context) {
	if coord, ok := s.popDirtyChunk(); ok {
		if err := s.sendChunkSummary(ctx, coord); err != nil {
			s.logger.Printf("load dirty chunk %v: %v", coord, err)
		}
		return
	}

	global, err := s.world.Region().LocalToGlobalChunk(s.chunkCursor)
	if err != nil {
		s.chunkCursor = world.LocalChunkIndex{}
		return
	}

	if err := s.sendChunkSummary(ctx, global); err != nil {
		s.logger.Printf("load chunk %v: %v", global, err)
		s.advanceChunkCursor()
		return
	}

	s.advanceChunkCursor()
}

func (s *Server) sendChunkSummary(ctx context.Context, coord world.ChunkCoord) error {
	chunk, err := s.world.Chunk(ctx, coord)
	if err != nil {
		return err
	}

	summary := network.ChunkSummary{
		ChunkX:     coord.X,
		ChunkY:     coord.Y,
		Version:    1,
		BlockCount: chunkBlockCount(chunk),
	}

	for _, endpoint := range s.cfg.Network.MainServerEndpoints {
		if err := s.net.Send(endpoint, network.MessageChunkSummary, summary); err != nil {
			s.logger.Printf("send chunk summary to %s: %v", endpoint, err)
		}
	}
	return nil
}

func (s *Server) advanceChunkCursor() {
	s.chunkCursor.X++
	if s.chunkCursor.X >= s.cfg.Chunk.ChunksPerAxis {
		s.chunkCursor.X = 0
		s.chunkCursor.Y++
		if s.chunkCursor.Y >= s.cfg.Chunk.ChunksPerAxis {
			s.chunkCursor.Y = 0
		}
	}
}

func (s *Server) onNeighborHello(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var msg network.NeighborHello
	if err := json.Unmarshal(env.Payload, &msg); err != nil {
		s.logger.Printf("neighbor hello decode: %v", err)
		return
	}
	origin := world.ChunkCoord{X: msg.RegionOriginX, Y: msg.RegionOriginY}
	var delta world.ChunkCoord
	if s.neighbors != nil {
		delta = s.neighbors.updateFromHello(addr.String(), msg.Listen, msg.ServerID, origin, msg.RegionSize)
	}
	region := s.world.Region()
	ack := network.NeighborAck{
		ServerID:      s.cfg.Server.ID,
		Listen:        s.cfg.Network.ListenUDP,
		RegionOriginX: region.Origin.X,
		RegionOriginY: region.Origin.Y,
		RegionSize:    region.ChunksPerAxis,
		DeltaX:        region.Origin.X - msg.RegionOriginX,
		DeltaY:        region.Origin.Y - msg.RegionOriginY,
		Timestamp:     time.Now().UTC(),
		Nonce:         msg.Nonce,
		Status:        "ok",
	}
	if err := s.net.Send(addr.String(), network.MessageNeighborAck, ack); err != nil {
		s.logger.Printf("neighbor ack send: %v", err)
	}
	s.logger.Printf("neighbor hello from %s via %s delta(%d,%d)", msg.ServerID, addr.String(), delta.X, delta.Y)
}

func (s *Server) onNeighborAck(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var ack network.NeighborAck
	if err := json.Unmarshal(env.Payload, &ack); err != nil {
		s.logger.Printf("neighbor ack decode: %v", err)
		return
	}
	origin := world.ChunkCoord{X: ack.RegionOriginX, Y: ack.RegionOriginY}
	if s.neighbors != nil {
		s.neighbors.updateFromAck(addr.String(), ack.Listen, ack.ServerID, origin, ack.RegionSize, ack.Nonce)
	}
	s.logger.Printf("neighbor ack from %s accepted=%s", ack.ServerID, ack.Status)
}

func (s *Server) onTransferRequest(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var req network.TransferRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		s.logger.Printf("transfer request decode: %v", err)
		return
	}
	ack := s.handleTransferRequest(req)
	if err := s.net.Send(addr.String(), network.MessageTransferAck, ack); err != nil {
		s.logger.Printf("transfer ack send: %v", err)
	}
}

func (s *Server) onTransferAck(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var ack network.TransferAck
	if err := json.Unmarshal(env.Payload, &ack); err != nil {
		s.logger.Printf("transfer ack decode: %v", err)
		return
	}
	s.logger.Printf("transfer ack entity %s accepted=%t from %s msg=%s", ack.EntityID, ack.Accepted, ack.FromServer, ack.Message)
	id := entities.ID(ack.EntityID)
	req, ok := s.inFlightTransfers[id]
	if !ok {
		return
	}
	delete(s.inFlightTransfers, id)

	if ack.Accepted {
		s.entities.Remove(id)
		delete(s.dirtyEntities, id)
		s.logger.Printf("migration: entity %s transferred to %s", ack.EntityID, ack.FromServer)
		return
	}

	if ent, ok := s.entities.Entity(id); ok {
		ent.SetAttribute("migration_pending", 0)
		s.recordDirtyEntity(ent)
		req.EntitySnapshot = ent.Snapshot()
		req.QueuedAt = time.Now()
		req.Nonce = 0
		s.migrationQueue.Enqueue(req)
	}
}

func (s *Server) handleTransferRequest(req network.TransferRequest) network.TransferAck {
	ack := network.TransferAck{
		EntityID:   req.EntityID,
		FromServer: s.cfg.Server.ID,
		ToServer:   req.FromServer,
		Nonce:      req.Nonce,
		Timestamp:  time.Now().UTC(),
	}
	targetChunk := world.ChunkCoord{X: req.GlobalChunkX, Y: req.GlobalChunkY}
	region := s.world.Region()
	if !region.ContainsGlobalChunk(targetChunk) {
		ack.Accepted = false
		ack.Message = "chunk outside region"
		return ack
	}
	if req.State.ID == "" {
		ack.Accepted = false
		ack.Message = "missing entity id"
		return ack
	}
	ent, err := s.buildEntityFromState(req.State, targetChunk)
	if err != nil {
		ack.Accepted = false
		ack.Message = err.Error()
		return ack
	}
	if err := s.entities.Add(ent); err != nil {
		ack.Accepted = false
		ack.Message = err.Error()
		return ack
	}
	ent.SetAttribute("migration_pending", 0)
	s.recordDirtyEntity(ent)
	ack.Accepted = true
	ack.Message = "accepted"
	s.logger.Printf("migration: entity %s received from %s", req.EntityID, req.FromServer)
	return ack
}

func (s *Server) buildEntityFromState(state network.EntityState, targetChunk world.ChunkCoord) (*entities.Entity, error) {
	pos := vec3FromSlice(state.Position)
	vel := vec3FromSlice(state.Velocity)
	ent := &entities.Entity{
		ID:   entities.ID(state.ID),
		Kind: entities.Kind(state.Kind),
		Chunk: entities.ChunkMembership{
			ServerID: s.cfg.Server.ID,
			Chunk:    targetChunk,
		},
		Position: pos,
		Velocity: vel,
		Stats: entities.Stats{
			MaxHP:     state.MaxHP,
			CurrentHP: state.HP,
		},
		Capabilities: entities.Capabilities{
			CanFly: state.CanFly,
			CanDig: state.CanDig,
		},
		Attributes: make(map[string]float64),
		LastTick:   time.Now(),
	}
	for k, v := range state.Attributes {
		ent.Attributes[k] = v
	}
	ent.UpdateChunk(s.cfg.Server.ID, targetChunk)
	return ent, nil
}

func vec3FromSlice(values []float64) entities.Vec3 {
	vec := entities.Vec3{}
	if len(values) > 0 {
		vec.X = values[0]
	}
	if len(values) > 1 {
		vec.Y = values[1]
	}
	if len(values) > 2 {
		vec.Z = values[2]
	}
	return vec
}

func (s *Server) onEntityQuery(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var query network.EntityQuery
	if err := json.Unmarshal(env.Payload, &query); err != nil {
		s.logger.Printf("entity query decode: %v", err)
		return
	}

	entities := s.entities.ByChunk(world.ChunkCoord{X: query.ChunkX, Y: query.ChunkY})
	result := network.EntityReply{
		ServerID: s.cfg.Server.ID,
		Entities: make([]network.EntityState, 0, len(entities)),
	}
	for _, ent := range entities {
		result.Entities = append(result.Entities, serializeEntity(ent))
	}

	if err := s.net.Send(addr.String(), network.MessageEntityReply, result); err != nil {
		s.logger.Printf("entity reply send: %v", err)
	}
}

func (s *Server) onPathRequest(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var req network.PathRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		s.logger.Printf("path request decode: %v", err)
		return
	}

	mode := pathfinding.ModeFromString(req.Mode)
	profile := pathfinding.DefaultProfile(mode)
	if req.Clearance > 0 {
		profile.Clearance = req.Clearance
	}
	if req.MaxClimb > 0 {
		profile.MaxClimb = req.MaxClimb
	}
	if req.MaxDrop > 0 {
		profile.MaxDrop = req.MaxDrop
	}

	start := world.BlockCoord{X: req.FromX, Y: req.FromY, Z: req.FromZ}
	goal := world.BlockCoord{X: req.ToX, Y: req.ToY, Z: req.ToZ}

	route := s.navigator.FindRoute(ctx, start, goal, profile)

	resp := network.PathResponse{
		EntityID: req.EntityID,
	}
	for _, coord := range route {
		resp.Route = append(resp.Route, network.BlockStep{X: coord.X, Y: coord.Y, Z: coord.Z})
	}

	if err := s.net.Send(addr.String(), network.MessagePathResponse, resp); err != nil {
		s.logger.Printf("path response send: %v", err)
	}
}

func (s *Server) onTransferClaim(ctx context.Context, addr *net.UDPAddr, env network.Envelope) {
	var claim network.TransferClaim
	if err := json.Unmarshal(env.Payload, &claim); err != nil {
		s.logger.Printf("transfer claim decode: %v", err)
		return
	}
	s.logger.Printf("transfer claim for entity %s from %s to %s", claim.EntityID, claim.From, claim.To)
}

func (s *Server) announceToMainServers() {
	payload := network.Hello{
		ServerID: s.cfg.Server.ID,
	}
	payload.Region.OriginX = s.cfg.Server.GlobalChunkOrigin.X
	payload.Region.OriginY = s.cfg.Server.GlobalChunkOrigin.Y
	payload.Region.Size = s.cfg.Chunk.ChunksPerAxis

	for _, endpoint := range s.cfg.Network.MainServerEndpoints {
		if err := s.net.Send(endpoint, network.MessageHello, payload); err != nil {
			s.logger.Printf("hello send to %s: %v", endpoint, err)
		}
	}
}

func serializeEntity(ent entities.Entity) network.EntityState {
	state := network.EntityState{
		ID:       string(ent.ID),
		Kind:     string(ent.Kind),
		ChunkX:   ent.Chunk.Chunk.X,
		ChunkY:   ent.Chunk.Chunk.Y,
		Position: []float64{ent.Position.X, ent.Position.Y, ent.Position.Z},
		Velocity: []float64{ent.Velocity.X, ent.Velocity.Y, ent.Velocity.Z},
		HP:       ent.Stats.CurrentHP,
		MaxHP:    ent.Stats.MaxHP,
		CanFly:   ent.Capabilities.CanFly,
		CanDig:   ent.Capabilities.CanDig,
		Voxels:   len(ent.Blocks),
	}
	if len(ent.Attributes) > 0 {
		state.Attributes = make(map[string]float64, len(ent.Attributes))
		for k, v := range ent.Attributes {
			state.Attributes[k] = v
		}
	}
	state.Dirty = ent.Dirty
	state.Dying = ent.Dying
	return state
}

func chunkBlockCount(chunk *world.Chunk) int {
	count := 0
	chunk.ForEachBlock(func(_ world.BlockCoord, block world.Block) bool {
		if block.Type != world.BlockAir {
			count++
		}
		return true
	})
	return count
}

func (s *Server) nextTransferNonce() uint64 {
	s.transferSeq++
	return s.transferSeq
}

func floorDiv(value, size int) int {
	if size <= 0 {
		return 0
	}
	if value >= 0 {
		return value / size
	}
	return -((-value - 1) / size) - 1
}

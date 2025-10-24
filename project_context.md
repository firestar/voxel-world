# Project Context

## Overview

The project implements a distributed voxel RTS engine. Chunk servers (Go) own 512×512×2048 voxel chunks, simulate entities, compute pathfinding, and manage block physics. Neighboring servers communicate via UDP handshakes to exchange migration and adjacency information. A central orchestrator (Go) launches and supervises chunk servers, exposes HTTP APIs for lookup, and maintains the world index so player clients can resolve which server covers a given coordinate.

## Key Components

- `chunk-server/`: Go module containing chunk server logic (voxel state, entities, migrations, networking).
- `central/`: Go module hosting the central orchestrator (cluster management, index, HTTP APIs).
- `chunk-server/cmd/pathclient`: Sample Go CLI that issues UDP path requests for manual testing.

## Current State

- Entity migration queues leverage neighbor handshakes to transfer entity state between servers; failed transfers are retried.
- Pathfinding responds to UDP `pathRequest` messages (see `cmd/pathclient`).
- Pathfinding now evaluates routes at the block level with unit-specific traversal profiles (ground, flying, underground) that enforce clearance, climb, and drop limits.
- Block-level pathfinding exposes profiler hooks to track heuristic usage, node expansion, and chunk cache behaviour for load testing.
- Central orchestrator configuration and README describe multi-server setups and lookup endpoints.
- Chunk servers prefetch chunk summaries for the entered chunk and its adjacent neighbors when entities cross chunk boundaries, reducing client hitching when players explore new regions.
- Movement simulation now runs on a dedicated worker that can scale across multiple threads and hands off entity/projectile velocity to neighboring servers when they exit a chunk.
- Automated tests cover movement engine timing (tick clamping and worker usage) alongside pathfinding constraints to ensure generated routes remain passable and avoid blocked endpoints.
- A configurable environment simulator advances day/night lighting, transitions between clear/rain/storm weather, and injects physics plus behaviour modifiers into entity updates; lighting is published through the world manager for downstream consumers.
- README documentation references orchestrator usage and notes that `project_context.md` must be kept current.

## File References

- ``$null``: Legacy placeholder file created accidentally; contains no meaningful data.
- `projectplan.md`: High-level roadmap outlining upcoming workstreams.
- `project_context.md`: Current project overview, components, next steps, and maintenance note.
- `chunk-server/go.mod`: Go module definition for the chunk server codebase.
- `chunk-server/README.md`: Documentation covering chunk server requirements, setup, orchestrator usage, and migration behaviour.
- `chunk-server/cmd/chunkserver/main.go`: Entrypoint for running a chunk server process (loads config, starts server loop).
- `chunk-server/cmd/pathclient/main.go`: Sample Go CLI client that issues UDP path requests to a chunk server.
- `chunk-server/internal/config/config.go`: Configuration structures, defaults, and validation for chunk servers.
- `chunk-server/internal/entities/entity.go`: Entity domain model (voxels, stats, capabilities) with physics helpers.
- `chunk-server/internal/entities/manager.go`: In-memory entity registry with chunk indexing, transfers, and mutation helpers.
- `chunk-server/internal/migration/queue.go`: Thread-safe queue for pending entity migrations.
- `chunk-server/internal/migration/types.go`: Migration-specific request/result types and metadata.
- `chunk-server/internal/network/protocol.go`: UDP protocol definitions (envelopes, message types, payload structs).
- `chunk-server/internal/network/protocol.go.bak`: Backup of a prior protocol definition for reference.
- `chunk-server/internal/network/server.go`: UDP server abstraction for registering handlers and sending messages.
- `chunk-server/internal/pathfinding/navigator.go`: Block-level A* navigator that enforces unit traversal constraints.
- `chunk-server/internal/server/delta.go`: Accumulator for coalescing voxel deltas before streaming.
- `chunk-server/internal/server/neighbor.go`: Neighbor management (handshakes, adjacency tracking, endpoints).
- `chunk-server/internal/server/server.go`: Core chunk server loop (ticks, migrations, streaming, request handlers).
- `chunk-server/internal/server/server.go.bak`: Backup of the chunk server implementation prior to latest edits.
- `chunk-server/internal/server/server.go.tmp`: Temporary scratch copy from earlier editing session.
- `chunk-server/internal/terrain/noise.go`: Deterministic terrain generation via value noise and mineral placement.
- `chunk-server/internal/world/chunk.go`: Sparse voxel chunk storage with block access helpers.
- `chunk-server/internal/world/damage.go`: Damage summary aggregation (block change tracking, collapse reporting).
- `chunk-server/internal/world/manager.go`: World manager orchestrating chunk loading, damage application, and stability cascades.
- `chunk-server/internal/world/region.go`: Utilities for mapping between global and local chunk/block coordinates.
- `chunk-server/internal/world/stability.go`: Block stability evaluation producing collapse reports.
- `central/central.yaml`: Sample orchestrator configuration (server addresses, chunk spans, world dimensions).
- `central/go.mod`: Go module definition for the central orchestrator.
- `central/go.sum`: Dependency lockfile for the central orchestrator (includes yaml.v3).
- `central/README.md`: Documentation for the central orchestrator (components, config, API).
- `central/cmd/central/main.go`: Entrypoint for running the central orchestrator service.
- `central/internal/cluster/manager.go`: Process lifecycle manager for chunk server instances.
- `central/internal/config/config.go`: Configuration loading/validation for the orchestrator.
- `central/internal/server/server.go`: HTTP server exposing health, chunk list, and coordinate lookup endpoints.
- `central/internal/worldmap/index.go`: Spatial index mapping chunk ranges to chunk server descriptors.

## Next Steps (excerpt)

- Add rate limiting/backpressure so voxel/ entity streaming does not overwhelm main servers.
- Profile block-level pathfinding heuristics and caching behaviour under heavy load.
- Integrate persistence (snapshot + replay) and a test harness for voxel updates.

## Maintenance Note

**Important:** After every project change, update `project_context.md` so this context accurately reflects the codebase, architecture, and priorities.

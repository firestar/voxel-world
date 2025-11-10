# Chunk Server

Chunk server prototype for the RTS voxel engine. Handles chunk ownership, entity state, pathfinding entry points, block stability, and UDP communication with main servers.

## Requirements Recap

- Each chunk is `256x256x1024` blocks.
- A chunk server owns a `32x32` chunk grid (default, configurable).
- World coordinates must be global; chunk servers know their XYZ span.
- Supports unlimited chunk grids by spinning up more servers; neighbor servers exchange adjacency info.
- Neighbor chunk servers discover one another via UDP handshakes to coordinate chunk transfers.
- Stream state to one or more main servers via UDP; main servers handle rendering and global game logic.
- Emit voxel change deltas in real time so explosions, digging, and collapses replicate to clients quickly.
- A central orchestration service (`../central`) can launch chunk servers and provide player routing information.
- Entities (including projectiles) are voxel-based and live within chunks; chunk server tracks entity memberships and exposes queries.
- Pathfinding uses A* across chunk graphs; chunk servers provide ingress/egress portals.
- Terrain generation uses global noise so chunk seams align.
- Supports block physics (support strength, collapses), digging, mining resources, and explosive interactions.

## Code Layout (initial)

- `cmd/chunkserver`: bootstrap executable for the chunk server daemon.
- `internal/config`: configuration loading (chunk geometry, tick rates, networking, economy).
- `internal/world`: chunk metadata, region bounds, block storage APIs, and stability analysis.
- `internal/terrain`: deterministic noise generator that materialises voxel columns and mineral pockets.
- `internal/entities`: entity definitions (units, projectiles, factories) and per chunk indexing.
- `internal/pathfinding`: chunk-level A* navigator for cross-server routing.
- `internal/network`: UDP protocol envelopes and lightweight message bus.
- `internal/server`: main orchestration loop that glues everything together.
- `internal/environment`: day/night and weather controller that feeds physics and lighting modifiers into the server loop.

## Running (local)

1. Install Go 1.21+.
2. From `chunk-server/`, run:

   ```bash
   go run ./cmd/chunkserver --config config.json
   ```

   If no configuration path is provided the defaults from `internal/config` are used.

### Running with the Central Orchestrator

For larger worlds you can delegate process management to the `central` orchestrator alongside the chunk server:

```bash
# Start the central orchestrator (this will launch configured chunk servers)
cd ../central
go run ./cmd/central --config central.yaml
```

Example `central.yaml` snippet configuring adjacent chunk servers:

```yaml
listen_address: 0.0.0.0
http_port: 28080
world:
  chunk_width: 256
  chunk_depth: 256
  chunk_height: 1024
chunk_servers:
  - id: chunk-east-0
    global_origin:
      chunk_x: 0
      chunk_y: 0
    chunk_span:
      chunks_x: 32
      chunks_y: 32
    args:
      - "--config"
      - "configs/chunk-east.json"
    listen_address: 127.0.0.1:19000
    http_address: http://127.0.0.1:19001
  - id: chunk-west-0
    global_origin:
      chunk_x: 32
      chunk_y: 0
    chunk_span:
      chunks_x: 32
      chunks_y: 32
    args:
      - "--config"
      - "configs/chunk-west.json"
    listen_address: 127.0.0.1:19100
    http_address: http://127.0.0.1:19101
```

Clients can query `GET /lookup?x=<blockX>&y=<blockY>` on the central service to discover the appropriate chunk server endpoint for a given world coordinate.

### Environment Simulation

The chunk server owns a lightweight environment simulator that advances a configurable day/night cycle and probabilistic weather patterns. The current state influences ambient lighting published by the world manager, physics coefficients used by the entity tickers (gravity, drag, friction), and per-entity behaviour attributes (visibility, morale, mobility throttling). Defaults provide a 20-minute solar cycle with clear, rain, and storm states that blend into entity physics automatically.

### Entity Migration

Chunk servers automatically queue entity migrations when units cross server boundaries. Once a neighbor handshake completes, the owning server serialises the entity state and issues a `transferRequest` to the adjacent chunk server. The receiving server reconstructs the entity, acknowledges the move, and the local server removes the migrated unit after a successful ack. Entities tagged with `migration_pending` pause simulation until the transfer completes or is retried.

## Sample Configuration

```json
{
  "server": {
    "id": "chunk-server-ny0",
    "globalChunkOrigin": {"x": 0, "y": 0},
    "tickRate": "33ms",
    "stateStreamRate": "200ms",
    "entityStreamRate": "50ms"
  },
  "chunk": {
    "width": 512,
    "depth": 512,
    "height": 2048,
    "chunksPerAxis": 32
  },
  "network": {
    "listenUdp": ":19000",
    "mainServerEndpoints": ["127.0.0.1:20000"],
    "handshakeTimeout": "3s",
    "keepAliveInterval": "5s",
    "discoveryInterval": "10s",
    "transferRetry": "2s",
    "neighborEndpoints": [
      {"chunkDelta": {"x": 32, "y": 0}, "endpoint": "127.0.0.1:19100"}
    ]
  },
  "environment": {
    "dayLength": "20m",
    "weatherMinDuration": "2m",
    "weatherMaxDuration": "5m",
    "stormChance": 0.15,
    "rainChance": 0.35,
    "windBase": 3.0,
    "windVariance": 5.0
  }
}
```

All duration values are parsed via Go's duration syntax (e.g. `"250ms"`, `"1s"`).

## Next Steps

- Add rate limiting/backpressure so voxel delta bursts don't overwhelm downstream consumers.
- Implement chunk streaming optimisations so arriving players receive neighbour chunk data eagerly.
- Expand pathfinding to operate on block-level nodes with traversal constraints per unit type.
- Integrate persistence (snapshot + replay) and a test harness for voxel updates.

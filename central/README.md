# Central Chunk Orchestrator

This module hosts the orchestration service responsible for spinning up, monitoring, and indexing the distributed chunk servers that own slices of the voxel world. Clients query this service to discover which chunk server covers a given XYZ coordinate.

## Components

- `cmd/central`: executable entrypoint for the orchestrator daemon.
- `internal/config`: configuration management (cluster layout, chunk server binaries/endpoints).
- `internal/cluster`: lifecycle manager for chunk server processes and health probes.
- `internal/worldmap`: spatial index mapping global chunk ranges to chunk server descriptors.
- `internal/server`: HTTP/gRPC APIs for clients + chunk servers, event loop integration.

## Responsibilities

- Boot each chunk server (local process or remote endpoint) from the cluster configuration.
- Maintain a registry of chunk server status, network endpoints, and chunk ranges.
- Provide client-facing lookup: given a world coordinate, return responsible chunk server connection info.
- Expose health metrics and allow admin operations (restart chunk server, drain chunk, etc.).

## Configuration

Example `central.yaml`:

```yaml
listen_address: 0.0.0.0
http_port: 28080
world:
  chunk_width: 512
  chunk_depth: 512
  chunk_height: 2048
cluster:
  default_binary: ../chunk-server/cmd/chunkserver/chunkserver
  data_root: ./data
  env:
    CHUNK_LOG_LEVEL: INFO
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
player_api:
  enabled: true
  base_url: https://api.example.com
```

## HTTP API

- `GET /healthz` – simple health check.
- `GET /chunk-servers` – list chunk servers with status and endpoints.
- `GET /lookup?x=<blockX>&y=<blockY>` – return chunk server covering the provided block coordinates.

## Next Steps

- Implement secure authentication between central and chunk servers.
- Add persistent storage (e.g., etcd/PostgreSQL) for world layout & cluster state.
- Integrate with the rendering/game master server for player session routing.

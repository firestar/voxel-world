# Voxel World

Voxel World is a distributed voxel RTS prototype composed of a central orchestration service, multiple chunk servers that own slices of the world, and a desktop client used to explore the terrain. This repository contains the source for each component along with default configuration files and development tooling.

## Components

- **Central orchestrator (`central/`)** – launches and monitors chunk servers, keeps their registry, and exposes discovery APIs for clients.
- **Chunk server (`chunk-server/`)** – maintains voxel data, entities, and simulation for a region of the world.
- **Desktop client (`client/`)** – Electron + Three.js application for rendering the world and interacting with the simulation.

## Downloading release binaries

Prebuilt binaries are published on the [GitHub Releases page](https://github.com/firestar/voxel-world/releases). Replace `<VERSION>` in the examples below with the desired tag (for example `v0.1.0`).

```bash
export VERSION=<VERSION>
```

### Central orchestrator

Download and run the Linux AMD64 release:

```bash
wget "https://github.com/firestar/voxel-world/releases/download/${VERSION}/central-linux-amd64" -O central
chmod +x central
./central --config central.yaml
```

The repository ships a sample configuration at [`central/central.yaml`](central/central.yaml). Copy it alongside the binary (or point to your own) before launching.

### Chunk server

Each chunk server instance needs its own configuration describing the portion of the world it owns. You can fetch the release binary and reuse the sample configs under [`chunk-server/configs/`](chunk-server/configs/):

```bash
wget "https://github.com/firestar/voxel-world/releases/download/${VERSION}/chunk-server-linux-amd64" -O chunk-server
chmod +x chunk-server
./chunk-server --config chunk-server/configs/chunk-east.json
```

When managed by the central orchestrator, make sure the paths referenced in [`central/central.yaml`](central/central.yaml) point to the downloaded chunk server binary and configuration files.

## Running from source

### Central orchestrator (Go)

```bash
cd central
go run ./cmd/central --config central.yaml
```

### Chunk server (Go)

```bash
cd chunk-server
go run ./cmd/chunkserver --config configs/chunk-east.json
```

### Desktop client (Electron + Vite)

```bash
cd client
npm install
npm run dev
```

The development script builds the Electron main/preload processes, starts the Vite renderer dev server, and launches Electron once everything is ready. Use `npm run start` to build and launch the packaged application locally.

## Next steps

- Wire authentication between central and chunk servers.
- Persist world configuration and chunk state for restart resiliency.
- Package signed installers for desktop client platforms.

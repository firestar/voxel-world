import dgram, { RemoteInfo } from 'dgram';
import { BrowserWindow } from 'electron';
import { URL } from 'url';
import {
  ChunkDeltaEvent,
  ChunkDeltaPayload,
  ChunkServerInfo,
  ChunkSummaryEvent,
  ChunkSummaryPayload,
  JoinResult,
  MessageType,
  encodeEnvelope,
  parseEnvelope
} from '@shared/protocol';

type ActiveStream = {
  socket: dgram.Socket;
  seq: number;
  server: ChunkServerInfo;
};

export class ChunkNetworkManager {
  private streams: Map<string, ActiveStream> = new Map();
  private window: BrowserWindow | null = null;

  setTarget(window: BrowserWindow) {
    this.window = window;
  }

  dispose() {
    for (const stream of this.streams.values()) {
      stream.socket.close();
    }
    this.streams.clear();
    this.window = null;
  }

  async connect(baseUrl: string): Promise<JoinResult> {
    const servers = await this.fetchChunkServers(baseUrl);
    if (!servers.length) {
      return { ok: false, message: 'No chunk servers reported by central.' };
    }
    this.resetStreams();
    for (const server of servers) {
      this.startStream(server);
    }
    return {
      ok: true,
      message: `Connected to central at ${baseUrl}`,
      servers
    };
  }

  private resetStreams() {
    for (const stream of this.streams.values()) {
      stream.socket.close();
    }
    this.streams.clear();
  }

  private async fetchChunkServers(baseUrl: string): Promise<ChunkServerInfo[]> {
    let normalized = baseUrl.trim();
    if (!/^https?:\/\//i.test(normalized)) {
      normalized = `http://${normalized}`;
    }
    let url: URL;
    try {
      url = new URL(normalized);
    } catch (err) {
      throw new Error(`Invalid central server URL: ${normalized}`);
    }
    url.pathname = '/chunk-servers';
    const response = await fetch(url.toString(), {
      headers: {
        Accept: 'application/json'
      }
    });
    if (!response.ok) {
      throw new Error(`Central server returned ${response.status}`);
    }
    const body = (await response.json()) as ChunkServerInfo[];
    return body;
  }

  private startStream(server: ChunkServerInfo) {
    const socket = dgram.createSocket('udp4');
    const stream: ActiveStream = {
      socket,
      seq: 1,
      server
    };
    this.streams.set(server.id, stream);

    socket.on('message', (msg: Buffer, rinfo: RemoteInfo) => {
      this.handlePacket(server, msg, rinfo);
    });
    socket.on('error', (err) => {
      console.error(`UDP error for ${server.id}`, err);
    });

    socket.bind(0, () => {
      const address = socket.address();
      console.log(`Listening for chunk data from ${server.id} on ${address.address}:${address.port}`);
      this.sendHello(stream);
    });
  }

  private sendHello(stream: ActiveStream) {
    const hostPort = this.splitHostPort(stream.server.listen_address);
    if (!hostPort) {
      console.warn(`Invalid listen address for ${stream.server.id}: ${stream.server.listen_address}`);
      return;
    }
    const [host, port] = hostPort;
    const payload = {
      serverId: 'voxel-world-client',
      region: {
        originX: 0,
        originY: 0,
        size: 1
      }
    };
    const buffer = encodeEnvelope('hello', payload, stream.seq++);
    stream.socket.send(buffer, port, host, (err) => {
      if (err) {
        console.error(`Failed to send hello to ${stream.server.id}`, err);
      }
    });
  }

  private splitHostPort(listen: string): [string, number] | null {
    const idx = listen.lastIndexOf(':');
    if (idx === -1) {
      return null;
    }
    const host = listen.slice(0, idx);
    const port = Number.parseInt(listen.slice(idx + 1), 10);
    if (!Number.isFinite(port)) {
      return null;
    }
    return [host, port];
  }

  private handlePacket(server: ChunkServerInfo, msg: Buffer, rinfo: RemoteInfo) {
    const env = parseEnvelope(msg);
    if (!env) {
      console.warn(`Discarding malformed packet from ${rinfo.address}:${rinfo.port}`);
      return;
    }
    switch (env.type as MessageType) {
      case 'chunkSummary': {
        const summary = env.payload as ChunkSummaryPayload;
        this.emitSummary(server.id, summary);
        break;
      }
      case 'chunkDelta': {
        const delta = env.payload as ChunkDeltaPayload;
        this.emitDelta(server.id, delta);
        break;
      }
      default:
        console.debug(`Received ${env.type} from ${server.id}`);
    }
  }

  private emitSummary(serverId: string, summary: ChunkSummaryPayload) {
    if (!this.window) {
      return;
    }
    const event: ChunkSummaryEvent = { serverId, summary };
    this.window.webContents.send('chunk-summary', event);
  }

  private emitDelta(serverId: string, delta: ChunkDeltaPayload) {
    if (!this.window) {
      return;
    }
    const event: ChunkDeltaEvent = { serverId, delta };
    this.window.webContents.send('chunk-delta', event);
  }
}

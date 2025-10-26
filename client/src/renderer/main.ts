import './styles.css';
import { GameScene } from './gameScene';
import type {
  ChunkServerInfo,
  ConnectionState,
  JoinResult
} from '@shared/protocol';

const serverInput = document.getElementById('server-url') as HTMLInputElement;
const joinButton = document.getElementById('join-button') as HTMLButtonElement;
const statusEl = document.getElementById('status') as HTMLParagraphElement;
const serverList = document.getElementById('server-list') as HTMLUListElement;
const logEl = document.getElementById('log') as HTMLDivElement;
const canvas = document.getElementById('game-canvas') as HTMLCanvasElement;

const scene = new GameScene(canvas);

function log(message: string) {
  const line = document.createElement('div');
  line.textContent = `[${new Date().toLocaleTimeString()}] ${message}`;
  logEl.prepend(line);
  const maxLines = 200;
  while (logEl.children.length > maxLines) {
    logEl.removeChild(logEl.lastChild!);
  }
}

function renderServers(servers: ChunkServerInfo[] | undefined) {
  serverList.innerHTML = '';
  if (!servers?.length) {
    const li = document.createElement('li');
    li.textContent = 'No servers connected';
    serverList.appendChild(li);
    return;
  }
  for (const server of servers) {
    const li = document.createElement('li');
    li.innerHTML = `
      <div class="server-row">
        <div class="server-id">${server.id}</div>
        <div class="server-status">${server.status}</div>
      </div>
      <div class="server-meta">UDP: ${server.listen_address}</div>
      <div class="server-meta">HTTP: ${server.http_address}</div>
    `;
    serverList.appendChild(li);
  }
}

async function handleJoin() {
  const url = serverInput.value.trim();
  if (!url) {
    statusEl.textContent = 'Please enter the central server URL.';
    return;
  }
  joinButton.disabled = true;
  statusEl.textContent = 'Connecting…';
  try {
    const result: JoinResult = await window.api.joinGame(url);
    if (!result.ok) {
      statusEl.textContent = result.message;
      log(`Join failed: ${result.message}`);
    } else {
      statusEl.textContent = result.message;
      renderServers(result.servers);
      log(`Connected to ${url}`);
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown join error';
    statusEl.textContent = message;
    log(`Join error: ${message}`);
  } finally {
    joinButton.disabled = false;
  }
}

joinButton.addEventListener('click', handleJoin);
serverInput.addEventListener('keydown', (event) => {
  if (event.key === 'Enter') {
    handleJoin().catch((err) => console.error(err));
  }
});

window.api.onConnectionState((state: ConnectionState) => {
  statusEl.textContent = state.message;
  if (state.servers) {
    renderServers(state.servers);
  }
  if (state.status === 'error') {
    log(`Connection error: ${state.message}`);
  }
});

window.api.onChunkSummary((event) => {
  scene.addChunkSummary(event);
  log(
    `Chunk summary ${event.summary.chunkX},${event.summary.chunkY} from ${event.serverId} with ${event.summary.blockCount} blocks`
  );
});

window.api.onChunkDelta((event) => {
  scene.applyChunkDelta(event);
  log(
    `Delta ${event.delta.blocks.length} blocks at ${event.delta.chunkX},${event.delta.chunkY} from ${event.serverId}`
  );
});

renderServers(undefined);
log('Client ready. Enter the central server URL and click Join.');

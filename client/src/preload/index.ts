import { contextBridge, ipcRenderer } from 'electron';
import {
  ChunkDeltaEvent,
  ChunkSummaryEvent,
  ConnectionState,
  JoinResult,
  WorldTimeState
} from '../shared/protocol';

type Listener<T> = (event: T) => void;

const connectionListeners = new Set<Listener<ConnectionState>>();
const summaryListeners = new Set<Listener<ChunkSummaryEvent>>();
const deltaListeners = new Set<Listener<ChunkDeltaEvent>>();
const timeListeners = new Set<Listener<WorldTimeState>>();

ipcRenderer.on('connection-state', (_event, state: ConnectionState) => {
  for (const listener of connectionListeners) {
    listener(state);
  }
});

ipcRenderer.on('chunk-summary', (_event, payload: ChunkSummaryEvent) => {
  for (const listener of summaryListeners) {
    listener(payload);
  }
});

ipcRenderer.on('chunk-delta', (_event, payload: ChunkDeltaEvent) => {
  for (const listener of deltaListeners) {
    listener(payload);
  }
});

ipcRenderer.on('world-time', (_event, payload: WorldTimeState) => {
  for (const listener of timeListeners) {
    listener(payload);
  }
});

contextBridge.exposeInMainWorld('api', {
  joinGame: (url: string): Promise<JoinResult> => ipcRenderer.invoke('join-game', url),
  onConnectionState: (listener: Listener<ConnectionState>) => {
    connectionListeners.add(listener);
    return () => connectionListeners.delete(listener);
  },
  onChunkSummary: (listener: Listener<ChunkSummaryEvent>) => {
    summaryListeners.add(listener);
    return () => summaryListeners.delete(listener);
  },
  onChunkDelta: (listener: Listener<ChunkDeltaEvent>) => {
    deltaListeners.add(listener);
    return () => deltaListeners.delete(listener);
  },
  onWorldTime: (listener: Listener<WorldTimeState>) => {
    timeListeners.add(listener);
    return () => timeListeners.delete(listener);
  }
});

import type {
  ChunkDeltaEvent,
  ChunkSummaryEvent,
  ConnectionState,
  JoinResult
} from '@shared/protocol';

declare global {
  interface Window {
    api: {
      joinGame: (url: string) => Promise<JoinResult>;
      onConnectionState: (
        listener: (state: ConnectionState) => void
      ) => () => void;
      onChunkSummary: (
        listener: (event: ChunkSummaryEvent) => void
      ) => () => void;
      onChunkDelta: (
        listener: (event: ChunkDeltaEvent) => void
      ) => () => void;
    };
  }
}

export {};

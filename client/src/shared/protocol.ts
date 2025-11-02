export type MessageType =
  | 'hello'
  | 'keepAlive'
  | 'chunkSummary'
  | 'chunkDelta'
  | 'entityUpdate'
  | 'entityQuery'
  | 'entityReply'
  | 'pathRequest'
  | 'pathResponse'
  | 'transferClaim'
  | 'transferRequest'
  | 'transferAck'
  | 'neighborHello'
  | 'neighborAck';

export interface Envelope<TPayload = unknown> {
  type: MessageType;
  timestamp: string;
  seq: number;
  payload: TPayload;
}

export interface ChunkSummaryPayload {
  chunkX: number;
  chunkY: number;
  version: number;
  blockCount: number;
}

export interface BlockChangePayload {
  x: number;
  y: number;
  z: number;
  type: string;
  material?: string;
  color?: string;
  texture?: string;
  hp: number;
  maxHp: number;
  reason: string;
  lightEmission?: number;
}

export interface ChunkDeltaPayload {
  serverId: string;
  chunkX: number;
  chunkY: number;
  seq: number;
  timestamp: string;
  blocks: BlockChangePayload[];
}

export interface ChunkServerInfo {
  id: string;
  status: string;
  started_at?: string;
  stopped_at?: string;
  listen_address: string;
  http_address: string;
  last_error?: string;
}

export interface JoinResult {
  ok: boolean;
  message: string;
  servers?: ChunkServerInfo[];
}

export interface ChunkSummaryEvent {
  serverId: string;
  summary: ChunkSummaryPayload;
}

export interface ChunkDeltaEvent {
  serverId: string;
  delta: ChunkDeltaPayload;
}

export interface ConnectionState {
  status: 'idle' | 'connecting' | 'connected' | 'error';
  message: string;
  servers?: ChunkServerInfo[];
}

export type DayPhase = 'dawn' | 'day' | 'dusk' | 'night';

export interface WorldTimeState {
  timeOfDay: number;
  progress: number;
  phase: DayPhase;
  sunAngle: number;
  sunPosition: { x: number; y: number; z: number };
  sunLightIntensity: number;
  ambientIntensity: number;
}

export function encodeEnvelope(
  type: MessageType,
  payload: unknown,
  seq: number
): Buffer {
  const envelope: Envelope = {
    type,
    timestamp: new Date().toISOString(),
    seq,
    payload
  };
  return Buffer.from(JSON.stringify(envelope), 'utf-8');
}

export function parseEnvelope(data: Buffer): Envelope | null {
  try {
    const env = JSON.parse(data.toString('utf-8')) as Envelope;
    return env;
  } catch (err) {
    return null;
  }
}

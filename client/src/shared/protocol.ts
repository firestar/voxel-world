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

export type BlockType =
  | 'unknown'
  | 'air'
  | 'solid'
  | 'unstable'
  | 'mineral'
  | 'explosive';

export const BlockTypeCodes = {
  Unknown: 0,
  Air: 1,
  Solid: 2,
  Unstable: 3,
  Mineral: 4,
  Explosive: 5
} as const;

export type BlockTypeCode = (typeof BlockTypeCodes)[keyof typeof BlockTypeCodes];

const BLOCK_TYPE_NAMES: ReadonlyArray<BlockType> = [
  'unknown',
  'air',
  'solid',
  'unstable',
  'mineral',
  'explosive'
];

export type ChangeReason = 'unknown' | 'damage' | 'destroy' | 'collapse';

export const ChangeReasonCodes = {
  Unknown: 0,
  Damage: 1,
  Destroy: 2,
  Collapse: 3
} as const;

export type ChangeReasonCode =
  (typeof ChangeReasonCodes)[keyof typeof ChangeReasonCodes];

const CHANGE_REASON_NAMES: ReadonlyArray<ChangeReason> = [
  'unknown',
  'damage',
  'destroy',
  'collapse'
];

export interface EncodedBlockChangePayload {
  x: number;
  y: number;
  z: number;
  type: BlockTypeCode;
  material?: string;
  color?: string;
  texture?: string;
  hp: number;
  maxHp: number;
  reason: ChangeReasonCode;
  lightEmission?: number;
}

export type BlockChangePayload = Omit<EncodedBlockChangePayload, 'type' | 'reason'> & {
  type: BlockType;
  reason: ChangeReason;
};

export interface EncodedChunkDeltaPayload {
  serverId: string;
  chunkX: number;
  chunkY: number;
  seq: number;
  timestamp: string;
  blocks: EncodedBlockChangePayload[];
}

export type ChunkDeltaPayload = Omit<EncodedChunkDeltaPayload, 'blocks'> & {
  blocks: BlockChangePayload[];
};

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

export function decodeBlockType(code: BlockTypeCode): BlockType {
  return BLOCK_TYPE_NAMES[code] ?? 'unknown';
}

export function decodeChangeReason(code: ChangeReasonCode): ChangeReason {
  return CHANGE_REASON_NAMES[code] ?? 'unknown';
}

export function decodeBlockChange(
  change: EncodedBlockChangePayload
): BlockChangePayload {
  return {
    ...change,
    type: decodeBlockType(change.type),
    reason: decodeChangeReason(change.reason)
  };
}

export function decodeChunkDeltaPayload(
  encoded: EncodedChunkDeltaPayload
): ChunkDeltaPayload {
  return {
    ...encoded,
    blocks: encoded.blocks.map(decodeBlockChange)
  };
}

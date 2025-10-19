package network

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	MessageHello            MessageType = "hello"
	MessageKeepAlive        MessageType = "keepAlive"
	MessageChunkSummary     MessageType = "chunkSummary"
	MessageChunkDelta       MessageType = "chunkDelta"
	MessageEntityUpdate     MessageType = "entityUpdate"
	MessageEntityQuery      MessageType = "entityQuery"
	MessageEntityReply      MessageType = "entityReply"
	MessagePathRequest      MessageType = "pathRequest"
	MessagePathResponse     MessageType = "pathResponse"
	MessageTransferClaim    MessageType = "transferClaim"
	MessageNeighborHello    MessageType = "neighborHello"
	MessageNeighborAck      MessageType = "neighborAck"
	MessageTransferRequest  MessageType = "transferRequest"
	MessageTransferAck      MessageType = "transferAck"
)

type Envelope struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Seq       uint64          `json:"seq"`
	Payload   json.RawMessage `json:"payload"`
}

type Hello struct {
	ServerID string `json:"serverId"`
	Region   struct {
		OriginX int `json:"originX"`
		OriginY int `json:"originY"`
		Size    int `json:"size"`
	} `json:"region"`
}

type KeepAlive struct {
	ServerID string    `json:"serverId"`
	Time     time.Time `json:"time"`
}

type ChunkSummary struct {
	ChunkX int `json:"chunkX"`
	ChunkY int `json:"chunkY"`
	Version uint64 `json:"version"`
	BlockCount int `json:"blockCount"`
}

type ChunkDelta struct {
	ServerID string        `json:"serverId"`
	ChunkX   int           `json:"chunkX"`
	ChunkY   int           `json:"chunkY"`
	Seq      uint64        `json:"seq"`
	Timestamp time.Time    `json:"timestamp"`
	Blocks   []BlockChange `json:"blocks"`
}

type BlockChange struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Z       int     `json:"z"`
	Type    string  `json:"type"`
	HP      float64 `json:"hp"`
	MaxHP   float64 `json:"maxHp"`
	Reason  string  `json:"reason"`
}

type NeighborHello struct {
	ServerID       string    `json:"serverId"`
	Listen         string    `json:"listen"`
	RegionOriginX  int       `json:"regionOriginX"`
	RegionOriginY  int       `json:"regionOriginY"`
	RegionSize     int       `json:"regionSize"`
	DeltaX         int       `json:"deltaX"`
	DeltaY         int       `json:"deltaY"`
	Timestamp      time.Time `json:"timestamp"`
	Nonce          uint64    `json:"nonce"`
}

type NeighborAck struct {
	ServerID       string    `json:"serverId"`
	Listen         string    `json:"listen"`
	RegionOriginX  int       `json:"regionOriginX"`
	RegionOriginY  int       `json:"regionOriginY"`
	RegionSize     int       `json:"regionSize"`
	DeltaX         int       `json:"deltaX"`
	DeltaY         int       `json:"deltaY"`
	Timestamp      time.Time `json:"timestamp"`
	Nonce          uint64    `json:"nonce"`
	Status         string    `json:"status"`
}

type EntityUpdate struct {
	EntityID string `json:"entityId"`
	ServerID string `json:"serverId"`
	ChunkX   int    `json:"chunkX"`
	ChunkY   int    `json:"chunkY"`
	State    EntityState `json:"state"`
}

type EntityQuery struct {
	ServerID string `json:"serverId"`
	ChunkX   int    `json:"chunkX"`
	ChunkY   int    `json:"chunkY"`
}

type EntityReply struct {
	ServerID string        `json:"serverId"`
	Entities []EntityState `json:"entities"`
}

type EntityState struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	ChunkX   int       `json:"chunkX"`
	ChunkY   int       `json:"chunkY"`
	Position []float64 `json:"position"`
	Velocity []float64 `json:"velocity"`
	HP       float64   `json:"hp"`
	MaxHP    float64   `json:"maxHp"`
	CanFly   bool      `json:"canFly"`
	CanDig   bool      `json:"canDig"`
	Voxels   int       `json:"voxels"`
	Attributes map[string]float64 `json:"attributes,omitempty"`
	Dirty    bool      `json:"dirty"`
	Dying    bool      `json:"dying"`
}

type EntityBatch struct {
	ServerID  string        `json:"serverId"`
	Seq       uint64        `json:"seq"`
	Timestamp time.Time     `json:"timestamp"`
	Entities  []EntityState `json:"entities"`
}

type PathRequest struct {
	EntityID string `json:"entityId"`
	FromX    int    `json:"fromChunkX"`
	FromY    int    `json:"fromChunkY"`
	ToX      int    `json:"toChunkX"`
	ToY      int    `json:"toChunkY"`
	Mode     string `json:"mode"`
}

type PathResponse struct {
	EntityID string `json:"entityId"`
	Route    []struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"route"`
}

type TransferClaim struct {
	EntityID string `json:"entityId"`
	From     string `json:"fromServer"`
	To       string `json:"toServer"`
}

type TransferRequest struct {
	EntityID      string    `json:"entityId"`
	FromServer    string    `json:"fromServer"`
	ToServer      string    `json:"toServer"`
	GlobalChunkX  int       `json:"globalChunkX"`
	GlobalChunkY  int       `json:"globalChunkY"`
	Reason        string    `json:"reason"`
	State        EntityState `json:"state"`
	Nonce         uint64    `json:"nonce"`
	Timestamp     time.Time `json:"timestamp"`
}

type TransferAck struct {
	EntityID  string    `json:"entityId"`
	FromServer string   `json:"fromServer"`
	ToServer   string   `json:"toServer"`
	Accepted   bool     `json:"accepted"`
	Message    string   `json:"message"`
	Nonce      uint64   `json:"nonce"`
	Timestamp  time.Time `json:"timestamp"`
}

func Encode(msg Envelope) ([]byte, error) {
	return json.Marshal(msg)
}

func Decode(data []byte) (Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	return env, err
}

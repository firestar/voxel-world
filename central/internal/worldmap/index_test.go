package worldmap

import (
	"testing"

	"central/internal/config"
)

func TestLoadFromConfigPopulatesEntries(t *testing.T) {
	cfg := &config.Config{
		ChunkServers: []config.ChunkServer{
			{
				ID:            "alpha",
				GlobalOrigin:  config.ChunkOrigin{ChunkX: 0, ChunkY: 0},
				ChunkSpan:     config.ChunkSpan{ChunksX: 2, ChunksY: 3},
				ListenAddress: "0.0.0.0:3000",
				HttpAddress:   "http://alpha",
			},
		},
	}

	idx := NewIndex()
	idx.LoadFromConfig(cfg)

	servers := idx.Servers()
	if len(servers) != 1 {
		t.Fatalf("Servers length = %d, want 1", len(servers))
	}
	got := servers[0]
	if got.ID != "alpha" || got.OriginChunkX != 0 || got.OriginChunkY != 0 {
		t.Fatalf("unexpected server info: %+v", got)
	}
	if got.ChunksX != 2 || got.ChunksY != 3 {
		t.Fatalf("unexpected chunk span: %+v", got)
	}
	if got.ListenAddress != "0.0.0.0:3000" || got.HTTPAddress != "http://alpha" {
		t.Fatalf("unexpected addresses: %+v", got)
	}
}

func TestLookupFindsMatchingServer(t *testing.T) {
	cfg := &config.Config{
		ChunkServers: []config.ChunkServer{
			{
				ID:           "alpha",
				GlobalOrigin: config.ChunkOrigin{ChunkX: 0, ChunkY: 0},
				ChunkSpan:    config.ChunkSpan{ChunksX: 1, ChunksY: 1},
			},
		},
	}
	idx := NewIndex()
	idx.LoadFromConfig(cfg)

	server, err := idx.Lookup(15, 15, 16, 16)
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if server.ID != "alpha" {
		t.Fatalf("Lookup returned server %q, want %q", server.ID, "alpha")
	}
}

func TestLookupNoMatchReturnsError(t *testing.T) {
	idx := NewIndex()
	idx.LoadFromConfig(&config.Config{})

	if _, err := idx.Lookup(32, 32, 16, 16); err == nil {
		t.Fatalf("Lookup() = nil, want error")
	}
}

func TestServersReturnsCopy(t *testing.T) {
	cfg := &config.Config{
		ChunkServers: []config.ChunkServer{{
			ID:           "alpha",
			GlobalOrigin: config.ChunkOrigin{ChunkX: 0, ChunkY: 0},
			ChunkSpan:    config.ChunkSpan{ChunksX: 1, ChunksY: 1},
		}},
	}
	idx := NewIndex()
	idx.LoadFromConfig(cfg)

	servers := idx.Servers()
	servers[0].ID = "modified"

	servers2 := idx.Servers()
	if servers2[0].ID != "alpha" {
		t.Fatalf("Servers returned slice is not a copy; got %q", servers2[0].ID)
	}
}

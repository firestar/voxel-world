package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"central/internal/config"
	"central/internal/worldmap"
)

func TestHandleLookupMissingParameters(t *testing.T) {
	srv := &Server{
		cfg:   &config.Config{},
		index: worldmap.NewIndex(),
	}

	req := httptest.NewRequest(http.MethodGet, "/lookup", nil)
	rr := httptest.NewRecorder()

	srv.handleLookup(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleLookupInvalidCoordinates(t *testing.T) {
	srv := &Server{
		cfg:   &config.Config{},
		index: worldmap.NewIndex(),
	}

	t.Run("invalid x", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/lookup?x=foo&y=1", nil)
		rr := httptest.NewRecorder()

		srv.handleLookup(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("invalid y", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/lookup?x=1&y=bar", nil)
		rr := httptest.NewRecorder()

		srv.handleLookup(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})
}

func TestHandleLookupNotFound(t *testing.T) {
	cfg := &config.Config{
		World: config.WorldConfig{
			ChunkWidth: 16,
			ChunkDepth: 16,
		},
	}
	srv := &Server{
		cfg:   cfg,
		index: worldmap.NewIndex(),
	}

	req := httptest.NewRequest(http.MethodGet, "/lookup?x=32&y=32", nil)
	rr := httptest.NewRecorder()

	srv.handleLookup(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestHandleLookupSuccess(t *testing.T) {
	cfg := &config.Config{
		World: config.WorldConfig{
			ChunkWidth: 16,
			ChunkDepth: 16,
		},
		ChunkServers: []config.ChunkServer{
			{
				ID: "alpha",
				GlobalOrigin: config.ChunkOrigin{
					ChunkX: 0,
					ChunkY: 0,
				},
				ChunkSpan: config.ChunkSpan{
					ChunksX: 2,
					ChunksY: 2,
				},
				ListenAddress: "127.0.0.1:3000",
				HttpAddress:   "127.0.0.1:3001",
			},
		},
	}
	idx := worldmap.NewIndex()
	idx.LoadFromConfig(cfg)
	srv := &Server{
		cfg:   cfg,
		index: idx,
	}

	req := httptest.NewRequest(http.MethodGet, "/lookup?x=16&y=16", nil)
	rr := httptest.NewRecorder()

	srv.handleLookup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	if !containsAll(body, []string{"\"ID\": \"alpha\"", "127.0.0.1:3000", "127.0.0.1:3001"}) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func containsAll(s string, substrs []string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestHandleTime(t *testing.T) {
	srv := &Server{
		cfg:   &config.Config{},
		cycle: newDayNightCycle(12*time.Minute, 9),
	}

	req := httptest.NewRequest(http.MethodGet, "/time", nil)
	rr := httptest.NewRecorder()

	srv.handleTime(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "sunPosition") {
		t.Fatalf("expected sunPosition in response, got %s", body)
	}
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"central/internal/cluster"
	"central/internal/config"
	"central/internal/worldmap"
)

type Server struct {
	cfg     *config.Config
	cluster *cluster.Manager
	index   *worldmap.Index
	httpSrv *http.Server
	logger  *log.Logger
}

func New(cfg *config.Config) (*Server, error) {
	manager, err := cluster.New(cfg)
	if err != nil {
		return nil, err
	}
	index := worldmap.NewIndex()
	index.LoadFromConfig(cfg)
	s := &Server{
		cfg:     cfg,
		cluster: manager,
		index:   index,
		logger:  log.New(log.Writer(), "central ", log.LstdFlags|log.Lmicroseconds),
	}
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()

	if err := s.cluster.StartAll(startCtx); err != nil {
		return err
	}
	defer s.cluster.Shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/chunk-servers", s.handleChunkServers)
	mux.HandleFunc("/lookup", s.handleLookup)

	addr := fmt.Sprintf("%s:%d", s.cfg.ListenAddress, s.cfg.HTTPPort)
	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("HTTP server listening on %s", addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleChunkServers(w http.ResponseWriter, r *http.Request) {
	servers := s.cluster.Processes()
	writeJSON(w, servers)
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	xStr := q.Get("x")
	yStr := q.Get("y")
	if xStr == "" || yStr == "" {
		http.Error(w, "x and y query parameters required", http.StatusBadRequest)
		return
	}
	x, err := strconv.Atoi(xStr)
	if err != nil {
		http.Error(w, "invalid x parameter", http.StatusBadRequest)
		return
	}
	y, err := strconv.Atoi(yStr)
	if err != nil {
		http.Error(w, "invalid y parameter", http.StatusBadRequest)
		return
	}

	info, err := s.index.Lookup(x, y, s.cfg.World.ChunkWidth, s.cfg.World.ChunkDepth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, info)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

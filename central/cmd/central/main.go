package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"central/internal/config"
	"central/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "central.yml", "configuration file for the central cluster orchestrator")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, cancel := signalContext(context.Background())
	defer cancel()

	s, err := server.New(cfg)
	if err != nil {
		log.Fatalf("initialise central server: %v", err)
	}

	if err := s.Run(ctx); err != nil {
		log.Fatalf("central server exited: %v", err)
	}
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer signal.Stop(signals)
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

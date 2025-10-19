package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chunkserver/internal/config"
	chunkserver "chunkserver/internal/server"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "path to chunk server configuration file")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	srv, err := chunkserver.New(cfg)
	if err != nil {
		log.Fatalf("initialise chunk server: %v", err)
	}

	ctx, cancel := signalContext()
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer signal.Stop(signals)
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}

		// Ensure the process terminates if shutdown stalls.
		time.AfterFunc(10*time.Second, func() {
			log.Printf("forced shutdown after timeout")
			os.Exit(1)
		})
	}()

	return ctx, cancel
}

package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"central/internal/config"
)

func TestStartAllMergesEnvironment(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "env.txt")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Env: map[string]string{
				"GLOBAL_FLAG": "cluster",
			},
		},
		ChunkServers: []config.ChunkServer{
			{
				ID:         "server-1",
				Executable: "/bin/sh",
				Args:       []string{"-c", "printf '%s\n%s\n%s\n%s\n' \"$GLOBAL_FLAG\" \"$SERVER_FLAG\" \"$CHUNK_LISTEN\" \"$OUTPUT_FILE\" > \"$OUTPUT_FILE\""},
				Env: map[string]string{
					"SERVER_FLAG": "chunk",
					"OUTPUT_FILE": outputPath,
				},
				ListenAddress: "127.0.0.1:9000",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr := New(cfg)
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}
	t.Cleanup(mgr.Shutdown)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(outputPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("env file %q was not created", outputPath)
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read env file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected env file contents: %q", string(data))
	}
	if lines[0] != "cluster" {
		t.Errorf("GLOBAL_FLAG = %q, want %q", lines[0], "cluster")
	}
	if lines[1] != "chunk" {
		t.Errorf("SERVER_FLAG = %q, want %q", lines[1], "chunk")
	}
	if lines[2] != "127.0.0.1:9000" {
		t.Errorf("CHUNK_LISTEN = %q, want %q", lines[2], "127.0.0.1:9000")
	}
	if lines[3] != outputPath {
		t.Errorf("OUTPUT_FILE = %q, want %q", lines[3], outputPath)
	}
}

func TestProcessesReportsExitStatus(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		ChunkServers: []config.ChunkServer{
			{
				ID:         "server-1",
				Executable: "/bin/sh",
				Args:       []string{"-c", "echo failing >&2; exit 12"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr := New(cfg)
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll() error = %v", err)
	}
	t.Cleanup(mgr.Shutdown)

	var infos []ProcessInfo
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		infos = mgr.Processes()
		if len(infos) > 0 && infos[0].Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(infos) == 0 {
		t.Fatalf("Processes() returned no entries")
	}
	info := infos[0]
	if info.Status != "stopped" {
		t.Fatalf("Status = %q, want %q", info.Status, "stopped")
	}
	if info.StoppedAt == nil {
		t.Fatalf("StoppedAt = nil, want non-nil")
	}
	if info.LastError == "" {
		t.Fatalf("LastError = empty, want non-empty")
	}
	if !strings.Contains(info.LastError, "exit status 12") {
		t.Fatalf("LastError = %q, want to contain exit status", info.LastError)
	}
}

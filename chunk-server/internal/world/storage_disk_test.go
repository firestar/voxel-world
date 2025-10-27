package world

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiskBlockStorageRotatesChunkFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunk.bin")

	storage, err := newDiskBlockStorage(path)
	if err != nil {
		t.Fatalf("newDiskBlockStorage: %v", err)
	}
	defer storage.Close()

	blocks := make([]Block, 4)
	for i := range blocks {
		blocks[i] = Block{Type: BlockSolid, Material: strings.Repeat("m", 64), Texture: strings.Repeat("t", 64)}
	}

	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(blocks); err != nil {
		t.Fatalf("encode blocks: %v", err)
	}

	originalLimit := maxChunkFileSize
	maxChunkFileSize = int64(9 + payload.Len())
	defer func() { maxChunkFileSize = originalLimit }()

	if err := storage.SaveColumn(0, blocks); err != nil {
		t.Fatalf("SaveColumn first: %v", err)
	}
	if err := storage.SaveColumn(1, blocks); err != nil {
		t.Fatalf("SaveColumn second: %v", err)
	}

	storage.mu.RLock()
	meta0 := storage.records[0]
	meta1 := storage.records[1]
	storage.mu.RUnlock()

	if meta0.part != 0 {
		t.Fatalf("expected first column in part 0, got %d", meta0.part)
	}
	if meta1.part != 1 {
		t.Fatalf("expected rotation to part 1, got %d", meta1.part)
	}

	if _, err := os.Stat(path + ".part1"); err != nil {
		t.Fatalf("expected rotated file to exist: %v", err)
	}

	column0, ok, err := storage.LoadColumn(0)
	if err != nil {
		t.Fatalf("LoadColumn part 0: %v", err)
	}
	if !ok {
		t.Fatalf("column 0 not found")
	}
	if !reflect.DeepEqual(column0, blocks) {
		t.Fatalf("column 0 mismatch")
	}

	column1, ok, err := storage.LoadColumn(1)
	if err != nil {
		t.Fatalf("LoadColumn part 1: %v", err)
	}
	if !ok {
		t.Fatalf("column 1 not found")
	}
	if !reflect.DeepEqual(column1, blocks) {
		t.Fatalf("column 1 mismatch")
	}
}

func TestDiskBlockStorageRejectsOversizedEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunk.bin")

	storage, err := newDiskBlockStorage(path)
	if err != nil {
		t.Fatalf("newDiskBlockStorage: %v", err)
	}
	defer storage.Close()

	blocks := []Block{{Type: BlockSolid, Material: strings.Repeat("m", 8)}}

	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(blocks); err != nil {
		t.Fatalf("encode blocks: %v", err)
	}

	originalLimit := maxChunkFileSize
	maxChunkFileSize = int64(9 + payload.Len() - 1)
	defer func() { maxChunkFileSize = originalLimit }()

	if err := storage.SaveColumn(0, blocks); err == nil {
		t.Fatalf("expected SaveColumn to fail for oversized entry")
	}
}

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

	payload, err := encodeColumnPayload(blocks)
	if err != nil {
		t.Fatalf("encode blocks: %v", err)
	}

	originalLimit := maxChunkFileSize
	maxChunkFileSize = int64(9 + len(payload))
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

func TestDiskBlockStoragePersistsIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunk.bin")

	storage, err := newDiskBlockStorage(path)
	if err != nil {
		t.Fatalf("newDiskBlockStorage: %v", err)
	}

	blocks := []Block{{Type: BlockSolid}}
	if err := storage.SaveColumn(7, blocks); err != nil {
		t.Fatalf("SaveColumn: %v", err)
	}

	if _, err := os.Stat(path + ".idx"); err != nil {
		t.Fatalf("expected index file to exist: %v", err)
	}

	reopened, err := newDiskBlockStorage(path)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	defer reopened.Close()

	column, ok, err := reopened.LoadColumn(7)
	if err != nil {
		t.Fatalf("LoadColumn: %v", err)
	}
	if !ok {
		t.Fatalf("expected column 7 to be present")
	}
	if !reflect.DeepEqual(column, blocks) {
		t.Fatalf("reloaded column mismatch")
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

	payload, err := encodeColumnPayload(blocks)
	if err != nil {
		t.Fatalf("encode blocks: %v", err)
	}

	originalLimit := maxChunkFileSize
	maxChunkFileSize = int64(9 + len(payload) - 1)
	defer func() { maxChunkFileSize = originalLimit }()

	if err := storage.SaveColumn(0, blocks); err == nil {
		t.Fatalf("expected SaveColumn to fail for oversized entry")
	}
}

func TestCompressColumnReducesRuns(t *testing.T) {
	block := Block{Type: BlockSolid, Material: "stone"}
	blocks := []Block{block, block, block, block}

	runs := compressColumn(blocks)
	if len(runs) != 1 {
		t.Fatalf("expected single run, got %d", len(runs))
	}
	if runs[0].Count != len(blocks) {
		t.Fatalf("expected run length %d, got %d", len(blocks), runs[0].Count)
	}
}

func TestEncodeColumnPayloadCompresses(t *testing.T) {
	block := Block{Type: BlockSolid, Material: strings.Repeat("stone", 16)}
	blocks := make([]Block, 128)
	for i := range blocks {
		blocks[i] = block
	}

	payload, err := encodeColumnPayload(blocks)
	if err != nil {
		t.Fatalf("encode column: %v", err)
	}

	var uncompressed bytes.Buffer
	encoding := columnEncoding{Version: columnEncodingVersion, Runs: compressColumn(blocks)}
	if err := gob.NewEncoder(&uncompressed).Encode(&encoding); err != nil {
		t.Fatalf("encode expected: %v", err)
	}

	if len(payload) >= uncompressed.Len() {
		t.Fatalf("expected compressed payload smaller than uncompressed (got %d vs %d)", len(payload), uncompressed.Len())
	}

	decoded, err := decodeColumnPayload(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !reflect.DeepEqual(decoded, blocks) {
		t.Fatalf("decoded blocks mismatch")
	}
}

func TestDecodeColumnPayloadLegacyFallback(t *testing.T) {
	legacy := []Block{{Type: BlockSolid}}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(legacy); err != nil {
		t.Fatalf("encode legacy: %v", err)
	}

	decoded, err := decodeColumnPayload(buf.Bytes())
	if err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
	if !reflect.DeepEqual(decoded, legacy) {
		t.Fatalf("legacy decode mismatch")
	}
}

package world

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

const (
	diskOpDelete byte = 0
	diskOpSet    byte = 1
)

var maxChunkFileSize int64 = 128 * 1024 * 1024

const (
	columnEncodingVersion = 1
)

func init() {
	gob.Register(map[string]any{})
	gob.Register(map[string]float64{})
}

type DiskStorageProvider struct {
	basePath string
	region   ServerRegion
}

// NewDiskStorageProvider creates a provider that persists chunk data beneath basePath.
func NewDiskStorageProvider(basePath string, region ServerRegion) *DiskStorageProvider {
	return &DiskStorageProvider{
		basePath: basePath,
		region:   region,
	}
}

func (p *DiskStorageProvider) NewStorage(key ChunkCoord, bounds Bounds, dim Dimensions) (BlockStorage, error) {
	path, err := p.chunkPath(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create chunk directory: %w", err)
	}
	return newDiskBlockStorage(path)
}

func (p *DiskStorageProvider) chunkPath(key ChunkCoord) (string, error) {
	local, err := p.region.GlobalToLocalChunk(key)
	if err != nil {
		return "", err
	}
	index := local.Y*p.region.ChunksPerAxis + local.X + 1
	dir := filepath.Join(p.basePath, strconv.Itoa(key.X), strconv.Itoa(key.Y))
	filename := fmt.Sprintf("chunk%02d.bin", index)
	return filepath.Join(dir, filename), nil
}

type diskRecordMeta struct {
	part   int
	offset int64
	size   uint32
}

type diskBlockStorage struct {
	basePath string
	mu       sync.RWMutex
	records  map[int]diskRecordMeta
	lastPart int
}

func newDiskBlockStorage(path string) (*diskBlockStorage, error) {
	storage := &diskBlockStorage{
		basePath: path,
		records:  make(map[int]diskRecordMeta),
	}
	if err := storage.ensureBaseFile(); err != nil {
		return nil, err
	}
	if err := storage.loadIndex(); err != nil {
		return nil, err
	}
	return storage, nil
}

func (s *diskBlockStorage) ensureBaseFile() error {
	f, err := os.OpenFile(s.basePath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open chunk file: %w", err)
	}
	return f.Close()
}

func (s *diskBlockStorage) partPath(part int) string {
	if part == 0 {
		return s.basePath
	}
	return fmt.Sprintf("%s.part%d", s.basePath, part)
}

func (s *diskBlockStorage) loadIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = make(map[int]diskRecordMeta)
	s.lastPart = 0

	header := make([]byte, 9)
	for part := 0; ; part++ {
		path := s.partPath(part)
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if part == 0 {
					// Base file should always exist due to ensureBaseFile.
					return nil
				}
				break
			}
			return fmt.Errorf("open chunk file %s: %w", path, err)
		}

		if err := s.scanPart(f, part, header); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close chunk file %s: %w", path, err)
		}
		s.lastPart = part
	}

	return nil
}

func (s *diskBlockStorage) scanPart(f *os.File, part int, header []byte) error {
	var offset int64
	for {
		if _, err := io.ReadFull(f, header); err != nil {
			if err == io.EOF {
				return nil
			}
			if err == io.ErrUnexpectedEOF {
				return fmt.Errorf("truncated chunk header in %s: %w", f.Name(), err)
			}
			return fmt.Errorf("read chunk header in %s: %w", f.Name(), err)
		}
		op := header[0]
		index := int(binary.LittleEndian.Uint32(header[1:5]))
		size := binary.LittleEndian.Uint32(header[5:9])
		recordOffset := offset
		offset += int64(len(header)) + int64(size)

		if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
			return fmt.Errorf("seek past payload in %s: %w", f.Name(), err)
		}
		if op == diskOpSet {
			s.records[index] = diskRecordMeta{part: part, offset: recordOffset, size: size}
		} else {
			delete(s.records, index)
		}
	}
}

func (s *diskBlockStorage) LoadColumn(index int) ([]Block, bool, error) {
	s.mu.RLock()
	meta, ok := s.records[index]
	s.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}

	header := make([]byte, 9)
	f, err := os.Open(s.partPath(meta.part))
	if err != nil {
		return nil, false, fmt.Errorf("open chunk file: %w", err)
	}
	defer f.Close()

	if _, err := f.ReadAt(header, meta.offset); err != nil {
		return nil, false, fmt.Errorf("read header at %d: %w", meta.offset, err)
	}
	if header[0] != diskOpSet {
		return nil, false, nil
	}
	size := binary.LittleEndian.Uint32(header[5:9])
	payload := make([]byte, size)
	if _, err := f.ReadAt(payload, meta.offset+int64(len(header))); err != nil {
		return nil, false, fmt.Errorf("read payload: %w", err)
	}
	blocks, err := decodeColumnPayload(payload)
	if err != nil {
		return nil, false, fmt.Errorf("decode column: %w", err)
	}
	return blocks, true, nil
}

func (s *diskBlockStorage) SaveColumn(index int, blocks []Block) error {
	payload, err := encodeColumnPayload(blocks)
	if err != nil {
		return fmt.Errorf("encode column: %w", err)
	}

	header := make([]byte, 9)
	header[0] = diskOpSet
	binary.LittleEndian.PutUint32(header[1:5], uint32(index))
	binary.LittleEndian.PutUint32(header[5:9], uint32(len(payload)))

	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.appendRecord(header, payload)
	if err != nil {
		return err
	}
	s.records[index] = meta
	return nil
}

func (s *diskBlockStorage) Delete(index int) error {
	header := make([]byte, 9)
	header[0] = diskOpDelete
	binary.LittleEndian.PutUint32(header[1:5], uint32(index))
	binary.LittleEndian.PutUint32(header[5:9], 0)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.appendRecord(header, nil); err != nil {
		return err
	}
	delete(s.records, index)
	return nil
}

func (s *diskBlockStorage) ForEach(fn func(index int, blocks []Block) bool) error {
	s.mu.RLock()
	metas := make([]struct {
		index int
		meta  diskRecordMeta
	}, 0, len(s.records))
	for idx, meta := range s.records {
		metas = append(metas, struct {
			index int
			meta  diskRecordMeta
		}{index: idx, meta: meta})
	}
	s.mu.RUnlock()

	sort.Slice(metas, func(i, j int) bool { return metas[i].index < metas[j].index })
	for _, entry := range metas {
		blocks, ok, err := s.LoadColumn(entry.index)
		if err != nil {
			log.Printf("disk block storage load index %d: %v", entry.index, err)
			continue
		}
		if !ok {
			continue
		}
		if !fn(entry.index, blocks) {
			break
		}
	}
	return nil
}

func (s *diskBlockStorage) Close() error {
	return nil
}

func (s *diskBlockStorage) appendRecord(header, payload []byte) (diskRecordMeta, error) {
	entrySize := int64(len(header) + len(payload))
	if entrySize > maxChunkFileSize {
		return diskRecordMeta{}, fmt.Errorf("chunk entry size %d exceeds max chunk file size %d", entrySize, maxChunkFileSize)
	}

	for {
		path := s.partPath(s.lastPart)
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return diskRecordMeta{}, fmt.Errorf("open chunk file %s: %w", path, err)
		}
		offset, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			f.Close()
			return diskRecordMeta{}, fmt.Errorf("seek chunk end: %w", err)
		}
		if offset+entrySize > maxChunkFileSize {
			if err := f.Close(); err != nil {
				return diskRecordMeta{}, fmt.Errorf("close chunk file %s: %w", path, err)
			}
			s.lastPart++
			continue
		}
		if _, err := f.Write(header); err != nil {
			f.Close()
			return diskRecordMeta{}, fmt.Errorf("write header: %w", err)
		}
		if len(payload) > 0 {
			if _, err := f.Write(payload); err != nil {
				f.Close()
				return diskRecordMeta{}, fmt.Errorf("write payload: %w", err)
			}
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return diskRecordMeta{}, fmt.Errorf("sync chunk file: %w", err)
		}
		if err := f.Close(); err != nil {
			return diskRecordMeta{}, fmt.Errorf("close chunk file %s: %w", path, err)
		}
		return diskRecordMeta{part: s.lastPart, offset: offset, size: uint32(len(payload))}, nil
	}
}

type columnRun struct {
	Count int
	Block Block
}

type columnEncoding struct {
	Version int
	Runs    []columnRun
}

func encodeColumnPayload(blocks []Block) ([]byte, error) {
	encoding := columnEncoding{Version: columnEncodingVersion}
	encoding.Runs = compressColumn(blocks)

	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(&encoding); err != nil {
		return nil, err
	}
	return payload.Bytes(), nil
}

func decodeColumnPayload(payload []byte) ([]Block, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var encoding columnEncoding
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&encoding); err == nil {
		switch encoding.Version {
		case columnEncodingVersion:
			return expandColumn(encoding.Runs), nil
		default:
			return nil, fmt.Errorf("unsupported column encoding version %d", encoding.Version)
		}
	}

	// Backwards compatibility: attempt to decode the legacy []Block payload.
	var legacy []Block
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&legacy); err != nil {
		return nil, err
	}
	return legacy, nil
}

func compressColumn(blocks []Block) []columnRun {
	if len(blocks) == 0 {
		return nil
	}

	runs := make([]columnRun, 0, 8)
	for _, block := range blocks {
		block = sanitizeBlock(block)
		n := len(runs)
		if n > 0 && blocksEqual(runs[n-1].Block, block) {
			runs[n-1].Count++
			continue
		}
		runs = append(runs, columnRun{Count: 1, Block: duplicateBlock(block)})
	}
	return runs
}

func expandColumn(runs []columnRun) []Block {
	if len(runs) == 0 {
		return nil
	}
	total := 0
	for _, run := range runs {
		total += run.Count
	}
	column := make([]Block, 0, total)
	for _, run := range runs {
		for i := 0; i < run.Count; i++ {
			column = append(column, duplicateBlock(run.Block))
		}
	}
	return column
}

func sanitizeBlock(block Block) Block {
	if len(block.ResourceYield) == 0 {
		block.ResourceYield = nil
	}
	if len(block.Metadata) == 0 {
		block.Metadata = nil
	}
	return block
}

func duplicateBlock(block Block) Block {
	clone := block
	if block.ResourceYield != nil {
		clone.ResourceYield = make(map[string]float64, len(block.ResourceYield))
		for k, v := range block.ResourceYield {
			clone.ResourceYield[k] = v
		}
	}
	if block.Metadata != nil {
		clone.Metadata = make(map[string]any, len(block.Metadata))
		for k, v := range block.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

func blocksEqual(a, b Block) bool {
	if a.Type != b.Type ||
		a.Material != b.Material ||
		a.Color != b.Color ||
		a.Texture != b.Texture ||
		a.HitPoints != b.HitPoints ||
		a.MaxHitPoints != b.MaxHitPoints ||
		a.ConnectingForce != b.ConnectingForce ||
		a.Weight != b.Weight ||
		a.LightEmission != b.LightEmission {
		return false
	}

	if len(a.ResourceYield) != len(b.ResourceYield) {
		return false
	}
	for k, v := range a.ResourceYield {
		if vb, ok := b.ResourceYield[k]; !ok || vb != v {
			return false
		}
	}

	if len(a.Metadata) != len(b.Metadata) {
		return false
	}
	for k, v := range a.Metadata {
		vb, ok := b.Metadata[k]
		if !ok {
			return false
		}
		if !metadataValueEqual(v, vb) {
			return false
		}
	}

	return true
}

func metadataValueEqual(a, b any) bool {
	switch va := a.(type) {
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case int:
		vb, ok := b.(int)
		return ok && va == vb
	case int64:
		vb, ok := b.(int64)
		return ok && va == vb
	case float64:
		vb, ok := b.(float64)
		return ok && va == vb
	case nil:
		return b == nil
	default:
		return fmt.Sprintf("%v", va) == fmt.Sprintf("%v", b)
	}
}

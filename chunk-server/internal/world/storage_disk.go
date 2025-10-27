package world

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
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
	offset int64
	size   uint32
}

type diskBlockStorage struct {
	file    *os.File
	mu      sync.RWMutex
	records map[int]diskRecordMeta
}

func newDiskBlockStorage(path string) (*diskBlockStorage, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open chunk file: %w", err)
	}
	storage := &diskBlockStorage{
		file:    f,
		records: make(map[int]diskRecordMeta),
	}
	if err := storage.loadIndex(); err != nil {
		f.Close()
		return nil, err
	}
	return storage, nil
}

func (s *diskBlockStorage) loadIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind chunk file: %w", err)
	}

	header := make([]byte, 9)
	var offset int64
	for {
		if _, err := io.ReadFull(s.file, header); err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				return fmt.Errorf("truncated chunk header: %w", err)
			}
			return fmt.Errorf("read chunk header: %w", err)
		}
		op := header[0]
		index := int(binary.LittleEndian.Uint32(header[1:5]))
		size := binary.LittleEndian.Uint32(header[5:9])
		recordOffset := offset
		offset += int64(len(header)) + int64(size)

		if _, err := s.file.Seek(int64(size), io.SeekCurrent); err != nil {
			return fmt.Errorf("seek past payload: %w", err)
		}
		if op == diskOpSet {
			s.records[index] = diskRecordMeta{offset: recordOffset, size: size}
		} else {
			delete(s.records, index)
		}
	}

	return nil
}

func (s *diskBlockStorage) LoadColumn(index int) ([]Block, bool, error) {
	s.mu.RLock()
	meta, ok := s.records[index]
	s.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}

	header := make([]byte, 9)
	if _, err := s.file.ReadAt(header, meta.offset); err != nil {
		return nil, false, fmt.Errorf("read header at %d: %w", meta.offset, err)
	}
	if header[0] != diskOpSet {
		return nil, false, nil
	}
	size := binary.LittleEndian.Uint32(header[5:9])
	payload := make([]byte, size)
	if _, err := s.file.ReadAt(payload, meta.offset+int64(len(header))); err != nil {
		return nil, false, fmt.Errorf("read payload: %w", err)
	}
	var blocks []Block
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&blocks); err != nil {
		return nil, false, fmt.Errorf("decode column: %w", err)
	}
	return blocks, true, nil
}

func (s *diskBlockStorage) SaveColumn(index int, blocks []Block) error {
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(blocks); err != nil {
		return fmt.Errorf("encode column: %w", err)
	}

	header := make([]byte, 9)
	header[0] = diskOpSet
	binary.LittleEndian.PutUint32(header[1:5], uint32(index))
	binary.LittleEndian.PutUint32(header[5:9], uint32(payload.Len()))

	s.mu.Lock()
	defer s.mu.Unlock()

	offset, err := s.file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek chunk end: %w", err)
	}
	if _, err := s.file.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := s.file.Write(payload.Bytes()); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync chunk file: %w", err)
	}
	s.records[index] = diskRecordMeta{offset: offset, size: uint32(payload.Len())}
	return nil
}

func (s *diskBlockStorage) Delete(index int) error {
	header := make([]byte, 9)
	header[0] = diskOpDelete
	binary.LittleEndian.PutUint32(header[1:5], uint32(index))
	binary.LittleEndian.PutUint32(header[5:9], 0)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek chunk end: %w", err)
	}
	if _, err := s.file.Write(header); err != nil {
		return fmt.Errorf("write delete header: %w", err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync chunk file: %w", err)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}

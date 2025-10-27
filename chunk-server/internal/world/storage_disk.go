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

	meta, err := s.appendRecord(header, payload.Bytes())
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

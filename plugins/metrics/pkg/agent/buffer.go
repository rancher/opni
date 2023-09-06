package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/golang/snappy"
	"github.com/google/uuid"
)

var ErrBufferNotFound = fmt.Errorf("buffer not found")

type ChunkBuffer interface {
	// Add blo cks until the value can be added to the buffer.
	Add(context.Context, string, ChunkMetadata) error

	// Get blocks until a value can be retrieved from the buffer.
	Get(context.Context, string) (ChunkMetadata, error)

	// Delete removes a buffer for the named task from the buffer.
	Delete(context.Context, string) error
}

type memoryBuffer struct {
	chanLocker sync.RWMutex
	chunkChan  map[string]chan ChunkMetadata
}

func NewMemoryBuffer() ChunkBuffer {
	return &memoryBuffer{
		chunkChan: make(map[string]chan ChunkMetadata),
	}
}

func (b *memoryBuffer) Add(_ context.Context, name string, meta ChunkMetadata) error {
	b.chanLocker.RLock()
	chunkChan, found := b.chunkChan[name]
	b.chanLocker.RUnlock()

	if !found {
		chunkChan = make(chan ChunkMetadata)

		b.chanLocker.Lock()
		b.chunkChan[name] = chunkChan
		b.chanLocker.Unlock()
	}

	chunkChan <- meta

	return nil
}

func (b *memoryBuffer) Get(ctx context.Context, name string) (ChunkMetadata, error) {
	b.chanLocker.RLock()
	chunkChan, found := b.chunkChan[name]
	b.chanLocker.RUnlock()

	if !found {
		return ChunkMetadata{}, ErrBufferNotFound
	}

	select {
	case <-ctx.Done():
		return ChunkMetadata{}, ctx.Err()
	case meta := <-chunkChan:
		return meta, nil
	}
}

func (b *memoryBuffer) Delete(_ context.Context, name string) error {
	b.chanLocker.Lock()
	delete(b.chunkChan, name)
	b.chanLocker.Unlock()

	return nil
}

type diskBuffer struct {
	dir string

	diskWriteLock sync.Mutex

	chanLocker sync.RWMutex
	chunkChans map[string]chan string
}

func NewDiskBuffer(dir string) (ChunkBuffer, error) {
	buffer := &diskBuffer{
		dir:        path.Join(BufferDir),
		chunkChans: make(map[string]chan string),
	}

	if err := os.MkdirAll(buffer.dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create buffer directory: %w", err)
	}

	if err := buffer.reconcileExistingChunks(); err != nil {
		return nil, err
	}

	return buffer, nil
}

func (b *diskBuffer) reconcileExistingChunks() error {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("could not reconcile existing chunks: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		chunkChan := make(chan string, 100)

		b.chanLocker.Lock()
		b.chunkChans[e.Name()] = chunkChan
		b.chanLocker.Unlock()

		subBufferDir := path.Join(b.dir, e.Name())
		subEntries, err := os.ReadDir(subBufferDir)
		if err != nil {
			return fmt.Errorf("could not reconcile existing chunks: %w", err)
		}

		for _, se := range subEntries {
			chunkChan <- path.Join(subBufferDir, se.Name())
		}
	}
	return nil
}

func (b *diskBuffer) Add(_ context.Context, name string, meta ChunkMetadata) error {
	b.chanLocker.RLock()
	chunkChan, found := b.chunkChans[name]
	b.chanLocker.RUnlock()

	if !found {
		chunkChan = make(chan string, 100)

		b.chanLocker.Lock()
		b.chunkChans[name] = chunkChan
		b.chanLocker.Unlock()
	}

	filePath := path.Join(b.dir, name, uuid.New().String())

	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("could not create buffer directory for target '%s': %w", name, err)
	}

	uncompressed, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("could not marshal chunk for buffer: %w", err)
	}

	compressed := snappy.Encode(nil, uncompressed)

	b.diskWriteLock.Lock()
	if err := os.WriteFile(filePath, compressed, 0644); err != nil {
		return fmt.Errorf("could not write chunk to buffer: %w", err)
	}
	b.diskWriteLock.Unlock()

	chunkChan <- filePath

	return nil
}

func (b *diskBuffer) Get(ctx context.Context, name string) (ChunkMetadata, error) {
	b.chanLocker.RLock()
	chunkChan, found := b.chunkChans[name]
	b.chanLocker.RUnlock()

	if !found {
		return ChunkMetadata{}, ErrBufferNotFound
	}

	select {
	case <-ctx.Done():
		return ChunkMetadata{}, ctx.Err()
	case path := <-chunkChan:
		compressed, err := os.ReadFile(path)
		if err != nil {
			return ChunkMetadata{}, fmt.Errorf("could not read chunk from buffer: %w", err)
		}

		uncompressed, err := snappy.Decode(nil, compressed)
		if err != nil {
			return ChunkMetadata{}, fmt.Errorf("could not decompress chunk from buffer: %w", err)
		}

		var meta ChunkMetadata
		if err := json.Unmarshal(uncompressed, &meta); err != nil {
			return ChunkMetadata{}, fmt.Errorf("could not unmarshal chunk from buffer: %w", err)
		}

		if err := os.Remove(path); err != nil {
			return ChunkMetadata{}, fmt.Errorf("could not remove chunk file from disk, data may linger on system longer than expected: %w", err)
		}

		return meta, nil
	}
}

func (b *diskBuffer) Delete(_ context.Context, name string) error {
	b.chanLocker.Lock()
	delete(b.chunkChans, name)
	b.chanLocker.Unlock()

	subBufferDir := path.Join(b.dir, name)
	if err := os.RemoveAll(subBufferDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove buffer directory: %w", err)
	}

	return nil
}

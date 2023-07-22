package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/golang/snappy"
	"github.com/google/uuid"
)

type Buffer[T any] interface {
	// Add blo cks until the value can be added to the buffer.
	Add(context.Context, T) error

	// Get blocks until a value can be retrieved from the buffer.
	Get(context.Context) (T, error)
}

type diskBuffer struct {
	dir   string
	queue chan string
}

// todo: reconcile pre-existing chunks (useful for pod restarts during import)
func NewDiskBuffer(dir string) (Buffer[WriteMetadata], error) {
	buffer := &diskBuffer{
		dir:   path.Join(BufferDir),
		queue: make(chan string, 100),
	}

	if err := os.MkdirAll(buffer.dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create buffer directory: %w", err)
	}

	return buffer, nil
}

func (b diskBuffer) Add(_ context.Context, meta WriteMetadata) error {
	// todo: will create a new directory for each target name which will not be cleaned up internally
	filePath := path.Join(b.dir, meta.Target, uuid.New().String())

	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("could not create buffer directory for target '%s': %w", meta.Target, err)
	}

	uncompressed, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("could not marshal chunk for buffer: %w", err)
	}

	compressed := snappy.Encode(nil, uncompressed)

	if err := os.WriteFile(filePath, compressed, 0644); err != nil {
		return fmt.Errorf("could not write chunk to buffer: %w", err)
	}

	b.queue <- filePath

	return nil
}

func (b diskBuffer) Get(_ context.Context) (WriteMetadata, error) {
	path := <-b.queue

	compressed, err := os.ReadFile(path)
	if err != nil {
		return WriteMetadata{}, fmt.Errorf("could not read chunk from buffer: %w", err)
	}

	uncompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		return WriteMetadata{}, fmt.Errorf("could not decompress chunk from buffer: %w", err)
	}

	var meta WriteMetadata
	if err := json.Unmarshal(uncompressed, &meta); err != nil {
		return WriteMetadata{}, fmt.Errorf("could not unmarshal chunk from buffer: %w", err)
	}

	if err := os.Remove(path); err != nil {
		return WriteMetadata{}, fmt.Errorf("could not remove chunk file from disk, data may linger on system longer than expected: %w", err)
	}

	return meta, nil
}

package agent

import "context"

type Buffer[T any] interface {
	// Add blo cks until the value can be added to the buffer.
	Add(context.Context, T) error

	// Get blocks until a value can be retrieved from the buffer.
	Get(context.Context) (T, error)
}

type memoryBuffer[T any] struct {
	ch chan T
}

func (b memoryBuffer[T]) Add(ctx context.Context, t T) error {
	b.ch <- t
	return nil
}

func (b memoryBuffer[T]) Get(ctx context.Context) (T, error) {
	return <-b.ch, nil
}

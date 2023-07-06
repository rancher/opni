package agent

type Buffer[T any] interface {
	// Add blo cks until the value can be added to the buffer.
	Add(T) error

	// Get blocks until a value can be retrieved from the buffer.
	Get() (T, error)
}

type memoryBuffer[T any] struct {
	ch chan T
}

func (b memoryBuffer[T]) Add(t T) error {
	b.ch <- t
	return nil
}

func (b memoryBuffer[T]) Get() (T, error) {
	return <-b.ch, nil
}

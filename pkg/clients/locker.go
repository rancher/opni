package clients

import (
	"sync"

	"github.com/samber/lo"
	"google.golang.org/grpc"
)

type Locker[T any] interface {
	// Use obtains exclusive ownership of the client, then calls the provided
	// function with the client. Ownership of the client is maintained until the
	// function returns.
	// It is safe to call Use if the Locker is nil; it will simply return false.
	Use(func(T)) bool
	// Close releases the client. After Close is called, Use() will return false.
	Close()
}

func NewLocker[T any](cc grpc.ClientConnInterface, builder func(grpc.ClientConnInterface) T) Locker[T] {
	return &clientLocker[T]{
		client: builder(cc),
	}
}

type clientLocker[T any] struct {
	sync.Mutex
	client T
	closed bool
}

func (cl *clientLocker[T]) Close() {
	if cl == nil {
		return
	}
	cl.Lock()
	defer cl.Unlock()
	if !cl.closed {
		cl.client = lo.Empty[T]()
		cl.closed = true
	}
}

func (cl *clientLocker[T]) Use(fn func(T)) bool {
	if cl == nil {
		return false
	}
	cl.Lock()
	defer cl.Unlock()
	if cl.closed {
		return false
	}
	fn(cl.client)
	return true
}

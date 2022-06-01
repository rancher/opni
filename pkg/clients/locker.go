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
	Use(func(T)) bool
	// Close releases the client. After Close is called, it is no longer safe to
	// call Use.
	Close()
}

func NewLocker[T any](cc grpc.ClientConnInterface, builder func(grpc.ClientConnInterface) T) Locker[T] {
	if cc == nil {
		return (*clientLocker[T])(nil)
	}
	return &clientLocker[T]{
		client: builder(cc),
	}
}

type clientLocker[T any] struct {
	sync.Mutex
	client T
}

func (cl *clientLocker[T]) Close() {
	if cl == nil {
		return
	}
	cl.Lock()
	defer cl.Unlock()
	cl.client = lo.Empty[T]()
}

func (cl *clientLocker[T]) Use(fn func(T)) bool {
	if cl == nil {
		return false
	}
	cl.Lock()
	defer cl.Unlock()
	fn(cl.client)
	return true
}

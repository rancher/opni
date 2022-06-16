package future

import (
	"context"
	"sync"
)

type Future[T any] interface {
	Get() T
	Set(T)
	IsSet() bool
	C() <-chan T
	GetContext(context.Context) (T, error)
}

type future[T any] struct {
	once   sync.Once
	object T
	wait   chan struct{}
}

func New[T any]() Future[T] {
	return &future[T]{
		wait: make(chan struct{}),
	}
}

func NewFromChannel[T any](ch <-chan T) Future[T] {
	f := New[T]()
	go func() {
		f.Set(<-ch)
	}()
	return f
}

func Instant[T any](value T) Future[T] {
	f := New[T]()
	f.Set(value)
	return f
}

func (f *future[T]) Set(object T) {
	f.once.Do(func() {
		f.object = object
		close(f.wait)
	})
}

func (f *future[T]) IsSet() bool {
	select {
	case <-f.wait:
		return true
	default:
		return false
	}
}

func (f *future[T]) Get() T {
	<-f.wait
	return f.object
}

func (f *future[T]) C() <-chan T {
	c := make(chan T, 1)
	if f.IsSet() {
		c <- f.object
	} else {
		go func() {
			<-f.wait
			c <- f.object
			close(c)
		}()
	}
	return c
}

func (f *future[T]) GetContext(ctx context.Context) (_ T, err error) {
	select {
	case <-f.wait:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}
	return f.object, nil
}

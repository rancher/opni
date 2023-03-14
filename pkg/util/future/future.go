package future

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
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

func Wait1[T any](f1 Future[T], callback func(T)) {
	go func() { callback(f1.Get()) }()
}
func Wait2[T, U any](f1 Future[T], f2 Future[U], callback func(T, U)) {
	go func() { callback(f1.Get(), f2.Get()) }()
}

func Wait3[T, U, V any](f1 Future[T], f2 Future[U], f3 Future[V], callback func(T, U, V)) {
	go func() { callback(f1.Get(), f2.Get(), f3.Get()) }()
}

func Wait4[T, U, V, W any](f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], callback func(T, U, V, W)) {
	go func() { callback(f1.Get(), f2.Get(), f3.Get(), f4.Get()) }()
}

func Wait5[T, U, V, W, X any](f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], f5 Future[X], callback func(T, U, V, W, X)) {
	go func() { callback(f1.Get(), f2.Get(), f3.Get(), f4.Get(), f5.Get()) }()
}

func Wait6[T, U, V, W, X, Y any](f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], f5 Future[X], f6 Future[Y], callback func(T, U, V, W, X, Y)) {
	go func() { callback(f1.Get(), f2.Get(), f3.Get(), f4.Get(), f5.Get(), f6.Get()) }()
}

func Wait7[T, U, V, W, X, Y, Z any](f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], f5 Future[X], f6 Future[Y], f7 Future[Z], callback func(T, U, V, W, X, Y, Z)) {
	go func() { callback(f1.Get(), f2.Get(), f3.Get(), f4.Get(), f5.Get(), f6.Get(), f7.Get()) }()
}

func Wait1Ctx[T any](ctx context.Context, f1 Future[T], callback func(T), errCallback func(error)) {
	var v1 T
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1)
	}()
}

func Wait2Ctx[T, U any](ctx context.Context, f1 Future[T], f2 Future[U], callback func(T, U), errCallback func(error)) {
	var v1 T
	var v2 U
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	eg.Go(func() (err error) { v2, err = f2.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1, v2)
	}()
}

func Wait3Ctx[T, U, V any](ctx context.Context, f1 Future[T], f2 Future[U], f3 Future[V], callback func(T, U, V), errCallback func(error)) {
	var v1 T
	var v2 U
	var v3 V
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	eg.Go(func() (err error) { v2, err = f2.GetContext(ctx); return })
	eg.Go(func() (err error) { v3, err = f3.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1, v2, v3)
	}()
}

func Wait4Ctx[T, U, V, W any](ctx context.Context, f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], callback func(T, U, V, W), errCallback func(error)) {
	var v1 T
	var v2 U
	var v3 V
	var v4 W
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	eg.Go(func() (err error) { v2, err = f2.GetContext(ctx); return })
	eg.Go(func() (err error) { v3, err = f3.GetContext(ctx); return })
	eg.Go(func() (err error) { v4, err = f4.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1, v2, v3, v4)
	}()
}

func Wait5Ctx[T, U, V, W, X any](ctx context.Context, f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], f5 Future[X], callback func(T, U, V, W, X), errCallback func(error)) {
	var v1 T
	var v2 U
	var v3 V
	var v4 W
	var v5 X
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	eg.Go(func() (err error) { v2, err = f2.GetContext(ctx); return })
	eg.Go(func() (err error) { v3, err = f3.GetContext(ctx); return })
	eg.Go(func() (err error) { v4, err = f4.GetContext(ctx); return })
	eg.Go(func() (err error) { v5, err = f5.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1, v2, v3, v4, v5)
	}()
}

func Wait6Ctx[T, U, V, W, X, Y any](ctx context.Context, f1 Future[T], f2 Future[U], f3 Future[V], f4 Future[W], f5 Future[X], f6 Future[Y], callback func(T, U, V, W, X, Y), errCallback func(error)) {
	var v1 T
	var v2 U
	var v3 V
	var v4 W
	var v5 X
	var v6 Y
	var eg errgroup.Group
	eg.Go(func() (err error) { v1, err = f1.GetContext(ctx); return })
	eg.Go(func() (err error) { v2, err = f2.GetContext(ctx); return })
	eg.Go(func() (err error) { v3, err = f3.GetContext(ctx); return })
	eg.Go(func() (err error) { v4, err = f4.GetContext(ctx); return })
	eg.Go(func() (err error) { v5, err = f5.GetContext(ctx); return })
	eg.Go(func() (err error) { v6, err = f6.GetContext(ctx); return })
	go func() {
		if err := eg.Wait(); err != nil {
			errCallback(err)
			return
		}
		callback(v1, v2, v3, v4, v5, v6)
	}()
}

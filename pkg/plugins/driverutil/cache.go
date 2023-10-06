package driverutil

import (
	"context"
	"sync"

	"github.com/samber/lo"
)

type Builder[T any] func(ctx context.Context, opts ...Option) (T, error)

type Cache[T any] interface {
	Register(name string, builder Builder[T])
	Unregister(name string)
	Get(name string) (Builder[T], bool)
	List() []string
	Range(func(name string, builder Builder[T]))
}

type driverCache[T any] struct {
	lock     sync.RWMutex
	builders map[string]Builder[T]
}

func (dc *driverCache[D]) Register(name string, builder Builder[D]) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.builders[name] = builder
}

func (dc *driverCache[D]) Unregister(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	delete(dc.builders, name)
}

func (dc *driverCache[D]) Get(name string) (Builder[D], bool) {
	dc.lock.RLock()
	defer dc.lock.RUnlock()

	builder, ok := dc.builders[name]
	return builder, ok
}

func (dc *driverCache[D]) List() []string {
	dc.lock.RLock()
	defer dc.lock.RUnlock()

	return lo.Keys(dc.builders)
}

func (dc *driverCache[D]) Range(fn func(name string, builder Builder[D])) {
	dc.lock.RLock()
	defer dc.lock.RUnlock()

	for name, builder := range dc.builders {
		fn(name, builder)
	}
}

func NewCache[T any]() Cache[T] {
	return &driverCache[T]{
		builders: make(map[string]Builder[T]),
	}
}

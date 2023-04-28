package driverutil

import (
	"context"
	"sync"

	"github.com/samber/lo"
)

type driver interface{}

type DriverBuilder[D driver] func(ctx context.Context, opts ...Option) (D, error)

type DriverCache[D driver] interface {
	Register(name string, builder DriverBuilder[D])
	Unregister(name string)
	Get(name string) (DriverBuilder[D], bool)
	List() []string
}

type driverCache[D driver] struct {
	lock     sync.Mutex
	builders map[string]DriverBuilder[D]
}

func (dc *driverCache[D]) Register(name string, builder DriverBuilder[D]) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.builders[name] = builder
}

func (dc *driverCache[D]) Unregister(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	delete(dc.builders, name)
}

func (dc *driverCache[D]) Get(name string) (DriverBuilder[D], bool) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	builder, ok := dc.builders[name]
	return builder, ok
}

func (dc *driverCache[D]) List() []string {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	return lo.Keys(dc.builders)
}

func NewDriverCache[D driver]() DriverCache[D] {
	return &driverCache[D]{
		builders: make(map[string]DriverBuilder[D]),
	}
}

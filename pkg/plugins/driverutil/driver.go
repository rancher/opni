package driverutil

import (
	"context"
	"sync"

	"github.com/samber/lo"
)

type DriverBuilder[D Driver] func(ctx context.Context, opts ...Option) (D, error)

type DriverCache[D Driver] interface {
	Register(name string, builder DriverBuilder[D])
	Unregister(name string)
	Get(name string) (DriverBuilder[D], bool)
	List() []string
}

type driverCache[D Driver] struct {
	lock     sync.RWMutex
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

func NewDriverCache[D Driver]() DriverCache[D] {
	return &driverCache[D]{
		builders: make(map[string]DriverBuilder[D]),
	}
}

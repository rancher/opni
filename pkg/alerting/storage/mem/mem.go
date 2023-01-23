package mem

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/storage/storage_opts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InMemoryRouterStore struct {
	mu      *sync.RWMutex
	routers map[string]routing.OpniRouting
}

func (i *InMemoryRouterStore) Get(ctx context.Context, key string, opts ...storage_opts.RequestOption) (routing.OpniRouting, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var t routing.OpniRouting
	if v, ok := i.routers[key]; !ok {
		return t, status.Error(codes.NotFound, "router not found")
	} else {
		return v, nil
	}
}

func (i *InMemoryRouterStore) Put(ctx context.Context, key string, value routing.OpniRouting) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.routers[key] = value.Clone()
	return nil
}

func (i *InMemoryRouterStore) ListKeys(ctx context.Context) ([]string, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var keys []string
	for k := range i.routers {
		keys = append(keys, k)
	}
	return keys, nil
}

func (i *InMemoryRouterStore) List(ctx context.Context, opts ...storage_opts.RequestOption) ([]routing.OpniRouting, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var routers []routing.OpniRouting
	for _, v := range i.routers {
		routers = append(routers, v.Clone())
	}
	return routers, nil
}

func (i *InMemoryRouterStore) Delete(ctx context.Context, key string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.routers, key)
	return nil
}

func NewInMemoryRouterStore() *InMemoryRouterStore {
	return &InMemoryRouterStore{
		mu:      &sync.RWMutex{},
		routers: make(map[string]routing.OpniRouting),
	}
}

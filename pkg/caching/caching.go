package caching

import (
	"time"

	"github.com/karlseguin/ccache"
	"github.com/rancher/opni/pkg/storage"
	"k8s.io/apimachinery/pkg/api/resource"
)

type InMemoryHttpTtlCache struct {
	cache *ccache.Cache

	maxAge time.Duration
}

func (i InMemoryHttpTtlCache) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryHttpTtlCache) Get(key string) (req []byte, ok bool) {
	item := i.cache.Get(key)
	if item == nil || item.Expired() {
		i.cache.Delete(key)
		return nil, false
	}

	return item.Value().([]byte), true
}

func (i InMemoryHttpTtlCache) Set(key string, req []byte) {
	i.cache.Set(key, req, i.maxAge)
}

func (i InMemoryHttpTtlCache) Delete(key string) {
	_ = i.cache.Delete(key)
}

var _ storage.HttpTtlCache = (*InMemoryHttpTtlCache)(nil)

func NewInMemoryHttpTtlCache(
	memoryLimit string,
	maxAge time.Duration,
) *InMemoryHttpTtlCache {
	q, err := resource.ParseQuantity(memoryLimit)
	if err != nil {
		panic(err)
	}
	memoryLimitInt := q.Value()

	ttlCache := ccache.New(ccache.Configure().MaxSize(memoryLimitInt).ItemsToPrune(15))
	return &InMemoryHttpTtlCache{
		cache:  ttlCache,
		maxAge: maxAge,
	}
}

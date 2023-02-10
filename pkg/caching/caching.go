package caching

import (
	"time"

	"github.com/karlseguin/ccache"
	"github.com/rancher/opni/pkg/storage"
	"k8s.io/apimachinery/pkg/api/resource"
)

type InMemoryHttpTtlCache struct {
	*ccache.Cache

	maxAge time.Duration
}

func (i InMemoryHttpTtlCache) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryHttpTtlCache) Get(key string) (req []byte, ok bool) {
	item := i.Cache.Get(key)
	if item == nil || item.Expired() {
		i.Cache.Delete(key)
		return nil, false
	}

	return item.Value().([]byte), true
}

func (i InMemoryHttpTtlCache) Set(key string, req []byte) {
	i.Cache.Set(key, req, i.maxAge)
}

func (i InMemoryHttpTtlCache) Delete(key string) {
	i.Cache.Delete(key)
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
		Cache:  ttlCache,
		maxAge: maxAge,
	}
}

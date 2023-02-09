package caching

import (
	"time"

	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/karlseguin/ccache"
	"github.com/rancher/opni/pkg/storage"
)

// CacheKeyer opt-in interface that proto messages implement.
// Used to determine if they are unique without hashing
type CacheKeyer interface {
	CacheKey() string
}

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

type InMemoryEntityCache struct {
	*ccache.Cache

	maxAge time.Duration
	// expired is a channel that is used to notify the cache
	// that an item has to be delete
}

func (i InMemoryEntityCache) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryEntityCache) Get(key string) (req proto.Message, ok bool) {
	item := i.Cache.Get(key)
	if item == nil || item.Expired() {
		i.Cache.Delete(key)
		return nil, false
	}

	return item.Value().(proto.Message), true
}

func (i InMemoryEntityCache) Set(key string, req proto.Message, ttl time.Duration) {
	i.Cache.Set(key, req, ttl)
}

func (i InMemoryEntityCache) Delete(key string) {
	i.Cache.Delete(key)
}

var _ storage.EntityCache = (*InMemoryEntityCache)(nil)

func NewInMemoryEntityCache(
	memoryLimit string,
	maxAge time.Duration,
) *InMemoryEntityCache {
	q, err := resource.ParseQuantity(memoryLimit)
	if err != nil {
		panic(err)
	}
	memoryLimitInt := q.Value()

	ttlCache := ccache.New(ccache.Configure().MaxSize(memoryLimitInt).ItemsToPrune(15))
	return &InMemoryEntityCache{
		Cache:  ttlCache,
		maxAge: maxAge,
	}
}

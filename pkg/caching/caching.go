package caching

import (
	"time"

	"google.golang.org/protobuf/proto"

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
	memoryLimitBytes int64,
	maxAge time.Duration,
) *InMemoryHttpTtlCache {
	ttlCache := ccache.New(ccache.Configure().MaxSize(memoryLimitBytes).ItemsToPrune(15))
	return &InMemoryHttpTtlCache{
		cache:  ttlCache,
		maxAge: maxAge,
	}
}

type InMemoryGrpcTtlCache struct {
	cache *ccache.Cache

	maxAge time.Duration
	// expired is a channel that is used to notify the cache
	// that an item has to be delete
}

func (i InMemoryGrpcTtlCache) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryGrpcTtlCache) Get(key string) (req proto.Message, ok bool) {
	item := i.cache.Get(key)
	if item == nil || item.Expired() {
		i.cache.Delete(key)
		return nil, false
	}

	return item.Value().(proto.Message), true
}

func (i InMemoryGrpcTtlCache) Set(key string, req proto.Message, ttl time.Duration) {
	i.cache.Set(key, req, ttl)
}

func (i InMemoryGrpcTtlCache) Delete(key string) {
	i.cache.Delete(key)
}

var _ storage.GrpcTtlCache = (*InMemoryGrpcTtlCache)(nil)

func NewInMemoryGrpcTtlCache(
	memoryLimitBytes int64,
	maxAge time.Duration,
) *InMemoryGrpcTtlCache {

	ttlCache := ccache.New(ccache.Configure().MaxSize(memoryLimitBytes).ItemsToPrune(15))
	return &InMemoryGrpcTtlCache{
		cache:  ttlCache,
		maxAge: maxAge,
	}
}

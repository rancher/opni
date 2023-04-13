package caching

import (
	"time"

	ccache "github.com/karlseguin/ccache/v3"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// CacheKeyer opt-in interface that proto messages implement.
// Used to determine if they are unique without hashing
type CacheKeyer interface {
	CacheKey() string
}

type InMemoryHttpTtlCache[T any] struct {
	cache *ccache.Cache[T]

	maxAge time.Duration
}

func (i InMemoryHttpTtlCache[T]) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryHttpTtlCache[T]) Get(key string) (req T, ok bool) {
	item := i.cache.Get(key)
	if item == nil || item.Expired() {
		var t T
		i.cache.Delete(key)
		return t, false
	}

	return item.Value(), true
}

func (i InMemoryHttpTtlCache[T]) Set(key string, req T) {
	i.cache.Set(key, req, i.maxAge)
}

func (i InMemoryHttpTtlCache[T]) Delete(key string) {
	_ = i.cache.Delete(key)
}

var _ storage.HttpTtlCache[any] = (*InMemoryHttpTtlCache[any])(nil)

func NewInMemoryHttpTtlCache(
	memoryLimitBytes int64,
	maxAge time.Duration,
) *InMemoryHttpTtlCache[[]byte] {
	ttlCache := ccache.New(ccache.Configure[[]byte]().MaxSize(memoryLimitBytes).ItemsToPrune(15))
	return &InMemoryHttpTtlCache[[]byte]{
		cache:  ttlCache,
		maxAge: maxAge,
	}
}

type InMemoryGrpcTtlCache[T any] struct {
	cache *ccache.Cache[T]

	maxAge time.Duration
}

func (i InMemoryGrpcTtlCache[T]) MaxAge() time.Duration {
	return i.maxAge
}

func (i InMemoryGrpcTtlCache[T]) Get(key string) (resp T, ok bool) {
	item := i.cache.Get(key)
	if item == nil || item.Expired() {
		var t T
		i.cache.Delete(key)
		return t, false
	}

	return item.Value(), true
}

func (i InMemoryGrpcTtlCache[T]) Set(key string, resp T, ttl time.Duration) {
	i.cache.Set(key, resp, ttl)
}

func (i InMemoryGrpcTtlCache[T]) Delete(key string) {
	i.cache.Delete(key)
}

var _ storage.GrpcTtlCache[any] = (*InMemoryGrpcTtlCache[any])(nil)

func NewInMemoryGrpcTtlCache(
	memoryLimitBytes int64,
	maxAge time.Duration,
) *InMemoryGrpcTtlCache[protoreflect.ProtoMessage] {

	ttlCache := ccache.New(ccache.Configure[protoreflect.ProtoMessage]().MaxSize(memoryLimitBytes).ItemsToPrune(15))
	return &InMemoryGrpcTtlCache[protoreflect.ProtoMessage]{
		cache:  ttlCache,
		maxAge: maxAge,
	}
}

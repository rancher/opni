package storage

import (
	"context"
	"sync"
)

type KeyValueStoreLocker[T any] interface {
	KeyValueStoreT[T]
	sync.Locker
}

type kvStoreLockerImpl[T any] struct {
	KeyValueStoreT[T]
	sync.Mutex
}

func NewKeyValueStoreLocker[T any](s KeyValueStoreT[T]) KeyValueStoreLocker[T] {
	return &kvStoreLockerImpl[T]{
		KeyValueStoreT: s,
	}
}

type ValueStoreLocker[T any] interface {
	ValueStoreT[T]
	sync.Locker
}

type valueStoreLockerImpl[T any] struct {
	ValueStoreT[T]
	sync.Locker
}

func NewValueStoreLocker[T any](s ValueStoreT[T], mutex ...sync.Locker) ValueStoreLocker[T] {
	var locker sync.Locker
	if len(mutex) == 0 {
		locker = &sync.Mutex{}
	} else {
		locker = mutex[0]
	}
	return &valueStoreLockerImpl[T]{
		ValueStoreT: s,
		Locker:      locker,
	}
}

type kvStorePrefixImpl[T any] struct {
	base   KeyValueStoreT[T]
	prefix string
}

func (s *kvStorePrefixImpl[T]) Put(ctx context.Context, key string, value T) error {
	return s.base.Put(ctx, s.prefix+key, value)
}

func (s *kvStorePrefixImpl[T]) Get(ctx context.Context, key string) (T, error) {
	return s.base.Get(ctx, s.prefix+key)
}

func (s *kvStorePrefixImpl[T]) Delete(ctx context.Context, key string) error {
	return s.base.Delete(ctx, s.prefix+key)
}

func (s *kvStorePrefixImpl[T]) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	return s.base.ListKeys(ctx, s.prefix+prefix)
}

func NewKeyValueStoreWithPrefix[T any](base KeyValueStoreT[T], prefix string) KeyValueStoreT[T] {
	return &kvStorePrefixImpl[T]{
		base:   base,
		prefix: prefix,
	}
}

type ValueStoreT[T any] interface {
	Put(ctx context.Context, value T) error
	Get(ctx context.Context) (T, error)
	Delete(ctx context.Context) error
}

type singleValueStoreImpl[T any] struct {
	base KeyValueStoreT[T]
	key  string
}

func (s *singleValueStoreImpl[T]) Put(ctx context.Context, value T) error {
	return s.base.Put(ctx, s.key, value)
}

func (s *singleValueStoreImpl[T]) Get(ctx context.Context) (T, error) {
	return s.base.Get(ctx, s.key)
}

func (s *singleValueStoreImpl[T]) Delete(ctx context.Context) error {
	return s.base.Delete(ctx, s.key)
}

func NewValueStore[T any](base KeyValueStoreT[T], key string) ValueStoreT[T] {
	return &singleValueStoreImpl[T]{
		base: base,
		key:  key,
	}
}

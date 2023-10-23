package kvutil

import (
	"context"
	"path"
	"strings"
	"sync"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
)

type KeyValueStoreLocker[T any] interface {
	storage.KeyValueStoreT[T]
	sync.Locker
}

type kvStoreLockerImpl[T any] struct {
	storage.KeyValueStoreT[T]
	sync.Mutex
}

func NewKeyValueStoreLocker[T any](s storage.KeyValueStoreT[T]) KeyValueStoreLocker[T] {
	return &kvStoreLockerImpl[T]{
		KeyValueStoreT: s,
	}
}

type ValueStoreLocker[T any] interface {
	storage.ValueStoreT[T]
	sync.Locker
}

type valueStoreLockerImpl[T any] struct {
	storage.ValueStoreT[T]
	sync.Locker
}

func NewValueStoreLocker[T any](s storage.ValueStoreT[T], mutex ...sync.Locker) ValueStoreLocker[T] {
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
	base   storage.KeyValueStoreT[T]
	prefix string
}

func (s *kvStorePrefixImpl[T]) Put(ctx context.Context, key string, value T, opts ...storage.PutOpt) error {
	return s.base.Put(ctx, s.prefix+key, value, opts...)
}

func (s *kvStorePrefixImpl[T]) Get(ctx context.Context, key string, opts ...storage.GetOpt) (T, error) {
	return s.base.Get(ctx, s.prefix+key, opts...)
}

func (s *kvStorePrefixImpl[T]) Watch(ctx context.Context, key string, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	c, err := s.base.Watch(ctx, s.prefix+key, opts...)
	if err != nil {
		return nil, err
	}
	out := make(chan storage.WatchEvent[storage.KeyRevision[T]], 1)
	go func() {
		defer close(out)
		for e := range c {
			// TODO: what the / doin
			if e.Current != nil {
				e.Current.SetKey(strings.TrimPrefix("/"+e.Current.Key(), s.prefix))
			}
			if e.Previous != nil {
				e.Previous.SetKey(strings.TrimPrefix("/"+e.Previous.Key(), s.prefix))
			}
			out <- e
		}
	}()
	return out, nil
}

func (s *kvStorePrefixImpl[T]) Delete(ctx context.Context, key string, opts ...storage.DeleteOpt) error {
	return s.base.Delete(ctx, s.prefix+key, opts...)
}

func (s *kvStorePrefixImpl[T]) ListKeys(ctx context.Context, prefix string, opts ...storage.ListOpt) ([]string, error) {
	return s.base.ListKeys(ctx, s.prefix+prefix, opts...)
}

func (s *kvStorePrefixImpl[T]) History(ctx context.Context, key string, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	return s.base.History(ctx, s.prefix+key, opts...)
}

func WithPrefix[T any](base storage.KeyValueStoreT[T], prefix string) storage.KeyValueStoreT[T] {
	return &kvStorePrefixImpl[T]{
		base:   base,
		prefix: prefix,
	}
}

type singleValueStoreImpl[T any] struct {
	base storage.KeyValueStoreT[T]
	key  string
}

func (s *singleValueStoreImpl[T]) Put(ctx context.Context, value T, opts ...storage.PutOpt) error {
	return s.base.Put(ctx, s.key, value, opts...)
}

func (s *singleValueStoreImpl[T]) Get(ctx context.Context, opts ...storage.GetOpt) (T, error) {
	return s.base.Get(ctx, s.key, opts...)
}

func (s *singleValueStoreImpl[T]) Watch(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	c, err := s.base.Watch(ctx, s.key, opts...)
	if err != nil {
		return nil, err
	}
	out := make(chan storage.WatchEvent[storage.KeyRevision[T]], 1)
	go func() {
		defer close(out)
		for e := range c {
			if e.Current != nil {
				e.Current.SetKey(path.Base(s.key))
			}
			if e.Previous != nil {
				e.Previous.SetKey(path.Base(s.key))
			}
			out <- e
		}
	}()
	return out, nil
}

func (s *singleValueStoreImpl[T]) Delete(ctx context.Context, opts ...storage.DeleteOpt) error {
	return s.base.Delete(ctx, s.key, opts...)
}

func (s *singleValueStoreImpl[T]) History(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	return s.base.History(ctx, s.key, opts...)
}

func WithKey[T any](base storage.KeyValueStoreT[T], key string) storage.ValueStoreT[T] {
	return &singleValueStoreImpl[T]{
		base: base,
		key:  key,
	}
}

type ValueStoreAdapter[T any] struct {
	PutFunc     func(ctx context.Context, value T, opts ...storage.PutOpt) error
	GetFunc     func(ctx context.Context, opts ...storage.GetOpt) (T, error)
	WatchFunc   func(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error)
	DeleteFunc  func(ctx context.Context, opts ...storage.DeleteOpt) error
	HistoryFunc func(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error)
}

func (s ValueStoreAdapter[T]) Put(ctx context.Context, value T, opts ...storage.PutOpt) error {
	return s.PutFunc(ctx, value, opts...)
}

func (s ValueStoreAdapter[T]) Get(ctx context.Context, opts ...storage.GetOpt) (T, error) {
	return s.GetFunc(ctx, opts...)
}

func (s ValueStoreAdapter[T]) Watch(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	return s.WatchFunc(ctx, opts...)
}

func (s ValueStoreAdapter[T]) Delete(ctx context.Context, opts ...storage.DeleteOpt) error {
	return s.DeleteFunc(ctx, opts...)
}

func (s ValueStoreAdapter[T]) History(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	return s.HistoryFunc(ctx, opts...)
}

type lockManagerPrefixImpl struct {
	base   storage.LockManager
	prefix string
}

func (s *lockManagerPrefixImpl) Locker(key string, opts ...lock.LockOption) storage.Lock {
	return s.base.Locker(s.prefix+key, opts...)
}

func LockManagerWithPrefix(base storage.LockManager, prefix string) storage.LockManager {
	return &lockManagerPrefixImpl{
		base:   base,
		prefix: prefix,
	}
}

func WithMessageCodec[T proto.Message](base storage.ValueStoreT[[]byte]) storage.ValueStoreT[T] {
	decodeKeyRevision := func(kr storage.KeyRevision[[]byte]) storage.KeyRevision[T] {
		if kr == nil {
			return nil
		}
		impl := &storage.KeyRevisionImpl[T]{
			K:    kr.Key(),
			V:    util.NewMessage[T](),
			Rev:  kr.Revision(),
			Time: kr.Timestamp(),
		}
		if err := proto.Unmarshal(kr.Value(), impl.V); err != nil {
			impl.V = lo.Empty[T]()
		}
		return impl
	}

	return ValueStoreAdapter[T]{
		PutFunc: func(ctx context.Context, value T, opts ...storage.PutOpt) error {
			bytes, err := proto.Marshal(value)
			if err != nil {
				return err
			}
			return base.Put(ctx, bytes, opts...)
		},
		GetFunc: func(ctx context.Context, opts ...storage.GetOpt) (T, error) {
			bytes, err := base.Get(ctx, opts...)
			if err != nil {
				return lo.Empty[T](), err
			}
			msg := util.NewMessage[T]()
			if err := proto.Unmarshal(bytes, msg); err != nil {
				return lo.Empty[T](), err
			}
			return msg, nil
		},
		WatchFunc: func(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
			c, err := base.Watch(ctx, opts...)
			if err != nil {
				return nil, err
			}
			out := make(chan storage.WatchEvent[storage.KeyRevision[T]], 1)
			go func() {
				defer close(out)
				for e := range c {
					typed := storage.WatchEvent[storage.KeyRevision[T]]{
						EventType: e.EventType,
						Current:   decodeKeyRevision(e.Current),
						Previous:  decodeKeyRevision(e.Previous),
					}
					out <- typed
				}
			}()
			return out, nil
		},
		DeleteFunc: func(ctx context.Context, opts ...storage.DeleteOpt) error {
			return base.Delete(ctx, opts...)
		},
		HistoryFunc: func(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
			resp, err := base.History(ctx, opts...)
			if err != nil {
				return nil, err
			}
			out := make([]storage.KeyRevision[T], len(resp))
			for i, kr := range resp {
				out[i] = decodeKeyRevision(kr)
			}
			return out, nil
		},
	}
}

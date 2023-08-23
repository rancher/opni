package inmemory

import (
	"context"
	"sync"

	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type inMemoryKeyValueStore[T any] struct {
	mu            sync.RWMutex
	newValueStore func(string) storage.ValueStoreT[T]
	keys          art.Tree
}

func NewKeyValueStore[T any](cloneFunc func(T) T) storage.KeyValueStoreT[T] {
	return &inMemoryKeyValueStore[T]{
		keys: art.New(),
		newValueStore: func(string) storage.ValueStoreT[T] {
			return NewValueStore[T](cloneFunc)
		},
	}
}

func NewCustomKeyValueStore[T any](newValueStore func(string) storage.ValueStoreT[T]) storage.KeyValueStoreT[T] {
	return &inMemoryKeyValueStore[T]{
		keys:          art.New(),
		newValueStore: newValueStore,
	}
}

// Delete implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) Delete(ctx context.Context, key string, opts ...storage.DeleteOpt) error {
	if err := validateKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.keys.Search(art.Key([]byte(key)))
	if !ok {
		return storage.ErrNotFound
	}
	vs := value.(storage.ValueStoreT[T])
	if err := vs.Delete(ctx, opts...); err != nil {
		return err
	}
	return nil
}

// Get implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) Get(ctx context.Context, key string, opts ...storage.GetOpt) (T, error) {
	if err := validateKey(key); err != nil {
		var zero T
		return zero, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.keys.Search(art.Key([]byte(key)))
	if !ok {
		var zero T
		return zero, storage.ErrNotFound
	}
	vs := value.(storage.ValueStoreT[T])
	return vs.Get(ctx, opts...)
}

// Watch implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) Watch(ctx context.Context, key string, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	matchingStores := []storage.ValueStoreT[T]{}
	m.keys.ForEachPrefix(art.Key([]byte(key)), func(node art.Node) (cont bool) {
		matchingStores = append(matchingStores, node.Value().(storage.ValueStoreT[T]))
		return true
	})
	aggregated := make(chan storage.WatchEvent[storage.KeyRevision[T]], len(matchingStores))
	for _, vs := range matchingStores {
		ch, err := vs.Watch(ctx, opts...)
		if err != nil {
			return nil, err
		}
		go func() {
			for event := range ch {
				aggregated <- event
			}
		}()
	}

	if ctx != context.Background() && ctx != context.TODO() {
		context.AfterFunc(ctx, func() {
			close(aggregated)
		})
	}
	return aggregated, nil
}

// History implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) History(ctx context.Context, key string, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.keys.Search(art.Key([]byte(key)))
	if !ok {
		return nil, storage.ErrNotFound
	}
	vs := value.(storage.ValueStoreT[T])
	elems, err := vs.History(ctx, opts...)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(elems); i++ {
		elems[i].(*storage.KeyRevisionImpl[T]).K = key
	}
	return elems, nil
}

// ListKeys implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) ListKeys(ctx context.Context, prefix string, opts ...storage.ListOpt) ([]string, error) {
	options := storage.ListKeysOptions{}
	options.Apply(opts...)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	m.keys.ForEachPrefix(art.Key([]byte(prefix)), func(node art.Node) (cont bool) {
		if node.Value() != nil {
			if _, err := node.Value().(storage.ValueStoreT[T]).Get(ctx); err != nil {
				return true
			}
			keys = append(keys, string(node.Key()))
		}
		return options.Limit == nil || int64(len(keys)) < *options.Limit
	})
	return keys, nil
}

// Put implements storage.KeyValueStoreT.
func (m *inMemoryKeyValueStore[T]) Put(ctx context.Context, key string, value T, opts ...storage.PutOpt) error {
	if err := validateKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	vs, ok := m.keys.Search(art.Key([]byte(key)))
	if !ok {
		vs = m.newValueStore(key)
		m.keys.Insert(art.Key([]byte(key)), vs)
	}
	return vs.(storage.ValueStoreT[T]).Put(ctx, value, opts...)
}

func validateKey(key string) error {
	if key == "" {
		return status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	return nil
}

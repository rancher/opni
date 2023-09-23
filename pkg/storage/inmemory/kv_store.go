package inmemory

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type inMemoryKeyValueStore[T any] struct {
	mu            sync.RWMutex
	newValueStore func(string) storage.ValueStoreT[T]
	keys          art.Tree
	watches       map[string]*activeWatch[T]
}

type activeWatch[T any] struct {
	matchesKey    func(string) bool
	ctx           context.Context
	cancel        context.CancelFunc
	watchedStores map[string]storage.ValueStoreT[T]
	ch            chan storage.WatchEvent[storage.KeyRevision[T]]
	wg            sync.WaitGroup
	addWatch      func(string, storage.ValueStoreT[T]) error
}

func NewKeyValueStore[T any](cloneFunc func(T) T) storage.KeyValueStoreT[T] {
	return &inMemoryKeyValueStore[T]{
		keys:    art.New(),
		watches: map[string]*activeWatch[T]{},
		newValueStore: func(string) storage.ValueStoreT[T] {
			return NewValueStore[T](cloneFunc)
		},
	}
}

func NewCustomKeyValueStore[T any](newValueStore func(string) storage.ValueStoreT[T]) storage.KeyValueStoreT[T] {
	return &inMemoryKeyValueStore[T]{
		keys:          art.New(),
		newValueStore: newValueStore,
		watches:       map[string]*activeWatch[T]{},
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
	options := storage.WatchOptions{}
	options.Apply(opts...)

	m.mu.Lock()
	defer m.mu.Unlock()

	matchingStores := map[string]storage.ValueStoreT[T]{}
	if options.Prefix {
		m.keys.ForEachPrefix(art.Key([]byte(key)), func(node art.Node) (cont bool) {
			if node.Value() != nil {
				matchingStores[string(node.Key())] = node.Value().(storage.ValueStoreT[T])
			}
			return true
		})
	} else {
		value, ok := m.keys.Search(art.Key([]byte(key)))
		if ok {
			matchingStores[key] = value.(storage.ValueStoreT[T])
		}
	}

	var vsOpts []storage.WatchOpt
	if options.Revision != nil {
		vsOpts = append(vsOpts, storage.WithRevision(*options.Revision))
	}

	aggregated := make(chan storage.WatchEvent[storage.KeyRevision[T]], 128)
	watch := activeWatch[T]{
		watchedStores: matchingStores,
		ch:            aggregated,
	}
	if options.Prefix {
		watch.matchesKey = func(k string) bool {
			return strings.HasPrefix(k, key)
		}
	} else {
		watch.matchesKey = func(k string) bool {
			return k == key
		}
	}
	watch.ctx, watch.cancel = context.WithCancel(context.WithoutCancel(ctx))

	addWatch := func(key string, vs storage.ValueStoreT[T]) error {
		ch, err := vs.Watch(watch.ctx, vsOpts...)
		if err != nil {
			return err
		}
		watch.wg.Add(1)
		go func() {
			defer watch.wg.Done()
			for {
				select {
				case <-watch.ctx.Done():
					return
				case event, ok := <-ch:
					if !ok {
						return
					}
					if event.Current != nil {
						event.Current.SetKey(key)
					}
					if event.Previous != nil {
						event.Previous.SetKey(key)
					}

					aggregated <- event
				}
			}
		}()
		return nil
	}
	watch.addWatch = addWatch

	var errs []error
	for key, vs := range matchingStores {
		if err := addWatch(key, vs); err != nil {
			errs = append(errs, err)
		}
	}
	if len(matchingStores) > 0 && len(errs) == len(matchingStores) {
		// only bail out if we know none of the watches started successfully,
		// otherwise it's not safe to close the channel
		close(aggregated)
		return nil, errors.Join(errs...)
	}

	watchId := uuid.NewString()
	m.watches[watchId] = &watch

	if ctx != context.Background() && ctx != context.TODO() {
		context.AfterFunc(ctx, func() {
			m.mu.Lock()
			delete(m.watches, watchId)
			m.mu.Unlock()

			watch.cancel()
			watch.wg.Wait()
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
	var vst storage.ValueStoreT[T]
	if !ok {
		vst = m.newValueStore(key)
		m.keys.Insert(art.Key([]byte(key)), vst)
		for _, watch := range m.watches {
			if watch.matchesKey(key) {
				if err := watch.addWatch(key, vst); err != nil {
					return err
				}
			}
		}
	} else {
		vst = vs.(storage.ValueStoreT[T])
	}
	return vst.Put(ctx, value, opts...)
}

func validateKey(key string) error {
	if key == "" {
		return status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	return nil
}

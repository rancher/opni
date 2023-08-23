package inmemory

import (
	"container/ring"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type valueStoreElement[T any] struct {
	revision  int64
	timestamp time.Time
	value     T
	deleted   bool
}

type inMemoryValueStore[T any] struct {
	ValueStoreOptions[T]
	lock      sync.RWMutex
	revision  int64
	values    *ring.Ring
	cloneFunc func(T) T
}

type ValueStoreOptions[T any] struct {
	onValueChanged []func(prev, value T)
}

type ValueStoreOption[T any] func(*ValueStoreOptions[T])

// Adds a listener function which will be called when the value is created,
// updated, or deleted, with the following considerations:
// - For create events, the previous value will be the zero value for T.
// - For delete events, the new value will be the zero value for T.
func OnValueChanged[T any](listener func(prev, value T)) ValueStoreOption[T] {
	return func(o *ValueStoreOptions[T]) {
		o.onValueChanged = append(o.onValueChanged, listener)
	}
}

// Returns a new value store for any type T that can be cloned using the provided clone function.
func NewValueStore[T any, O ValueStoreOption[T]](cloneFunc func(T) T, opts ...O) storage.ValueStoreT[T] {
	options := ValueStoreOptions[T]{}
	for _, opt := range opts {
		opt(&options)
	}

	return &inMemoryValueStore[T]{
		ValueStoreOptions: options,
		values:            ring.New(64),
		cloneFunc:         cloneFunc,
	}
}

func (s *inMemoryValueStore[T]) isEmptyLocked() bool {
	return s.revision == 0
}

func (s *inMemoryValueStore[T]) Put(_ context.Context, value T, opts ...storage.PutOpt) error {
	options := storage.PutOptions{}
	options.Apply(opts...)

	s.lock.Lock()
	defer s.lock.Unlock()
	if options.Revision != nil {
		if *options.Revision == 0 {
			if s.revision != 0 && !s.values.Value.(*valueStoreElement[T]).deleted {
				return fmt.Errorf("%w: expected value not to exist (requested revision 0)", storage.ErrConflict)
			}
		} else if *options.Revision != s.revision {
			return fmt.Errorf("%w: revision mismatch: %v (requested) != %v (actual)", storage.ErrConflict, *options.Revision, s.revision)
		}
	}
	previous := s.values.Value
	s.revision++
	next := s.values.Next()
	next.Value = &valueStoreElement[T]{
		revision:  s.revision,
		timestamp: time.Now(),
		value:     value,
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = s.revision
	}
	s.values = next

	var prevValue T
	if previous != nil {
		prevValue = s.cloneFunc(previous.(*valueStoreElement[T]).value)
	}
	var wg sync.WaitGroup
	for _, listener := range s.onValueChanged {
		listener := listener
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener(prevValue, value)
		}()
	}
	wg.Wait()

	return nil
}

func (s *inMemoryValueStore[T]) Get(_ context.Context, opts ...storage.GetOpt) (T, error) {
	options := storage.GetOptions{}
	options.Apply(opts...)

	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.isEmptyLocked() {
		var zero T
		if options.Revision != nil && *options.Revision != 0 {
			return zero, status.Errorf(codes.OutOfRange, "revision %d is a future revision", *options.Revision)
		}
		return zero, storage.ErrNotFound
	}

	var found *valueStoreElement[T]
	if options.Revision != nil {
		if s.revision == *options.Revision {
			found = s.values.Value.(*valueStoreElement[T])
		} else if *options.Revision > s.revision {
			var zero T
			return zero, status.Errorf(codes.OutOfRange, "revision %d is a future revision", *options.Revision)
		} else {
			for elem := s.values.Prev(); elem != s.values; elem = elem.Prev() {
				if elem.Value == nil {
					break
				}
				v := elem.Value.(*valueStoreElement[T])
				if v.deleted {
					continue
				}
				if v.revision == *options.Revision {
					found = v
					break
				}
			}
		}
	} else {
		found = s.values.Value.(*valueStoreElement[T])
	}
	var zero T
	if found == nil {
		return zero, storage.ErrNotFound
	}
	if found.deleted {
		return zero, storage.ErrNotFound
	}

	if options.RevisionOut != nil {
		*options.RevisionOut = found.revision
	}

	return s.cloneFunc(found.value), nil
}

func (s *inMemoryValueStore[T]) Watch(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	panic("not implemented")
}

func (s *inMemoryValueStore[T]) Delete(_ context.Context, opts ...storage.DeleteOpt) error {
	options := storage.DeleteOptions{}
	options.Apply(opts...)

	s.lock.Lock()
	defer s.lock.Unlock()
	if s.isEmptyLocked() {
		return storage.ErrNotFound
	}
	if options.Revision != nil && *options.Revision != s.revision {
		return fmt.Errorf("%w: revision mismatch: %v (requested) != %v (actual)", storage.ErrConflict, *options.Revision, s.revision)
	}
	if s.values.Value.(*valueStoreElement[T]).deleted {
		return storage.ErrNotFound
	}
	previous := s.values.Value
	s.revision++
	next := s.values.Next()
	next.Value = &valueStoreElement[T]{
		revision:  s.revision,
		timestamp: time.Now(),
		deleted:   true,
	}
	s.values = next

	prevValue := s.cloneFunc(previous.(*valueStoreElement[T]).value)
	var wg sync.WaitGroup
	for _, listener := range s.onValueChanged {
		listener := listener
		wg.Add(1)
		go func() {
			defer wg.Done()
			var zero T
			listener(prevValue, zero)
		}()
	}
	wg.Wait()
	return nil
}

func (s *inMemoryValueStore[T]) History(_ context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	options := storage.HistoryOptions{}
	options.Apply(opts...)

	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.isEmptyLocked() {
		return nil, storage.ErrNotFound
	}
	var revisions []storage.KeyRevision[T]
	current := s.values
	if options.Revision != nil {
		for ; current != s.values.Next(); current = current.Prev() {
			if current.Value == nil {
				return nil, storage.ErrNotFound
			}
			curElem := current.Value.(*valueStoreElement[T])
			if curElem.deleted {
				continue
			}
			if curElem.revision == *options.Revision {
				break
			}
		}
	}

	if current != nil && current.Value != nil && current.Value.(*valueStoreElement[T]).deleted {
		return nil, storage.ErrNotFound
	}

	for ; current != s.values.Next(); current = current.Prev() {
		if current.Value == nil {
			break
		}
		curElem := current.Value.(*valueStoreElement[T])
		if curElem.deleted {
			break
		}
		rev := &storage.KeyRevisionImpl[T]{
			Rev:  curElem.revision,
			Time: curElem.timestamp,
		}
		if options.IncludeValues {
			rev.V = s.cloneFunc(curElem.value)
		}
		revisions = append(revisions, rev)
	}
	return lo.Reverse(revisions), nil
}

package inmemory

import (
	"container/ring"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	gsync "github.com/kralicky/gpkg/sync"
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
	lock      sync.RWMutex
	revision  int64
	values    *ring.Ring
	cloneFunc func(T) T
	watches   gsync.Map[string, func(storage.WatchEvent[storage.KeyRevision[T]])]
}

// Returns a new value store for any type T that can be cloned using the provided clone function.
func NewValueStore[T any](cloneFunc func(T) T) storage.ValueStoreT[T] {
	return &inMemoryValueStore[T]{
		values:    ring.New(64),
		cloneFunc: cloneFunc,
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
	timestamp := time.Now()
	next.Value = &valueStoreElement[T]{
		revision:  s.revision,
		timestamp: timestamp,
		value:     value,
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = s.revision
	}
	s.values = next

	var prevValue *valueStoreElement[T]
	var eventType storage.WatchEventType
	if previous == nil || previous.(*valueStoreElement[T]).deleted {
		eventType = storage.WatchEventCreate
	} else {
		eventType = storage.WatchEventUpdate
		prevValue = previous.(*valueStoreElement[T])
	}
	var wg sync.WaitGroup
	s.watches.Range(func(_ string, listener func(storage.WatchEvent[storage.KeyRevision[T]])) bool {
		wg.Add(1)
		go func() {
			defer wg.Done()
			current := &storage.KeyRevisionImpl[T]{
				V:    s.cloneFunc(value),
				Rev:  s.revision,
				Time: timestamp,
			}
			switch eventType {
			case storage.WatchEventCreate:
				listener(storage.WatchEvent[storage.KeyRevision[T]]{
					EventType: eventType,
					Current:   current,
				})
			case storage.WatchEventUpdate:
				listener(storage.WatchEvent[storage.KeyRevision[T]]{
					EventType: eventType,
					Current:   current,
					Previous: &storage.KeyRevisionImpl[T]{
						V:    s.cloneFunc(prevValue.value),
						Rev:  prevValue.revision,
						Time: prevValue.timestamp,
					},
				})
			}
		}()
		return true
	})
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
	options := storage.WatchOptions{}
	options.Apply(opts...)

	updateC := make(chan storage.WatchEvent[storage.KeyRevision[T]], 64)
	buffer := make(chan storage.WatchEvent[storage.KeyRevision[T]], 8)

	s.lock.RLock()
	current := s.values
	if options.Revision != nil {
		// walk back until we find the starting revision
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
	}

	// if there is a previous value for the target revision, keep track of it
	var previous storage.KeyRevision[T]
	if current.Prev().Value != nil {
		prevValue := current.Prev().Value.(*valueStoreElement[T])
		if !prevValue.deleted {
			previous = &storage.KeyRevisionImpl[T]{
				V:    s.cloneFunc(prevValue.value),
				Rev:  prevValue.revision,
				Time: prevValue.timestamp,
			}
		} else {
			previous = nil
		}
	}

	// walk forward until we reach the current value and write the events
	for ; current != s.values.Next(); current = current.Next() {
		if current.Value == nil {
			continue
		}
		curElem := current.Value.(*valueStoreElement[T])
		if curElem.deleted {
			updateC <- storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: storage.WatchEventDelete,
				Previous:  previous,
			}
			previous = nil
			continue
		}
		current := &storage.KeyRevisionImpl[T]{
			V:    s.cloneFunc(curElem.value),
			Rev:  curElem.revision,
			Time: curElem.timestamp,
		}
		if previous == nil {
			updateC <- storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: storage.WatchEventCreate,
				Current:   current,
			}
		} else {
			updateC <- storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: storage.WatchEventUpdate,
				Current:   current,
				Previous:  previous,
			}
		}
		previous = current
	}
	s.lock.RUnlock()

	// watch for future updates
	id := uuid.NewString()

	s.watches.Store(id, func(ev storage.WatchEvent[storage.KeyRevision[T]]) {
		select {
		case <-ctx.Done():
		case buffer <- ev:
		}
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.watches.Delete(id)
				for {
					select {
					case v := <-buffer:
						updateC <- v
					default:
						close(updateC)
						return
					}
				}
			case ev := <-buffer:
				updateC <- ev
			}
		}
	}()

	return updateC, nil
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

	prevValue := previous.(*valueStoreElement[T])
	var wg sync.WaitGroup
	s.watches.Range(func(_ string, listener func(storage.WatchEvent[storage.KeyRevision[T]])) bool {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener(storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: storage.WatchEventDelete,
				Previous: &storage.KeyRevisionImpl[T]{
					V:    prevValue.value,
					Rev:  prevValue.revision,
					Time: prevValue.timestamp,
				},
			})
		}()
		return true
	})
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

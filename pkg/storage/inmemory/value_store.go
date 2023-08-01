package inmemory

import (
	"container/ring"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type valueStoreElement[T any] struct {
	revision  int64
	timestamp time.Time
	value     T
	deleted   bool
}

type inMemoryValueStore[T any] struct {
	lock           sync.RWMutex
	revision       int64
	values         *ring.Ring
	onValueChanged []func(prev, value T)
	cloneFunc      func(T) T
}

func NewValueStore[T any](cloneFunc func(T) T, listeners ...func(prev, value T)) storage.ValueStoreT[T] {
	return &inMemoryValueStore[T]{
		values:         ring.New(64),
		onValueChanged: listeners,
		cloneFunc:      cloneFunc,
	}
}

func NewProtoValueStore[T proto.Message](listeners ...func(prev, value T)) storage.ValueStoreT[T] {
	return NewValueStore(util.ProtoClone, listeners...)
}

func (s *inMemoryValueStore[T]) isEmptyLocked() bool {
	return s.revision == 0
}

func (s *inMemoryValueStore[T]) Put(ctx context.Context, value T, opts ...storage.PutOpt) error {
	options := storage.PutOptions{}
	options.Apply(opts...)

	s.lock.Lock()
	defer s.lock.Unlock()
	if options.Revision != nil && *options.Revision != s.revision {
		return fmt.Errorf("%w: revision mismatch: %v (requested) != %v (actual)", storage.ErrConflict, *options.Revision, s.revision)
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
	for _, listener := range s.onValueChanged {
		go listener(prevValue, value)
	}

	return nil
}

func (s *inMemoryValueStore[T]) Get(ctx context.Context, opts ...storage.GetOpt) (T, error) {
	options := storage.GetOptions{}
	options.Apply(opts...)

	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.isEmptyLocked() {
		var zero T
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

func (s *inMemoryValueStore[T]) Delete(ctx context.Context, opts ...storage.DeleteOpt) error {
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
	for _, listener := range s.onValueChanged {
		var zero T
		go listener(prevValue, zero)
	}
	return nil
}

func (s *inMemoryValueStore[T]) History(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
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

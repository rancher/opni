package storage

import (
	"context"
	"errors"
	"time"

	"go.uber.org/atomic"
)

var (
	ErrEventChannelClosed = errors.New("event channel closed")
	ErrObjectDeleted      = errors.New("object deleted")
)

type watchContext[T any] struct {
	base context.Context

	eventC <-chan WatchEvent[T]
	done   chan struct{}
	err    *atomic.Error
}

// Returns a context that listens on a watch event channel and closes its
// Done channel when the object is deleted. This context should have exclusive
// read access to the event channel to avoid missing events.
func NewWatchContext[T any](
	base context.Context,
	eventC <-chan WatchEvent[T],
) context.Context {
	cc := &watchContext[T]{
		base:   base,
		eventC: eventC,
		done:   make(chan struct{}),
		err:    atomic.NewError(nil),
	}
	go cc.watch()
	return cc
}

func (cc *watchContext[T]) watch() {
	defer close(cc.done)
	for {
		select {
		case <-cc.base.Done():
			cc.err.Store(cc.base.Err())
			return
		case event, ok := <-cc.eventC:
			if !ok {
				cc.err.Store(ErrEventChannelClosed)
				return
			}
			if event.EventType == WatchEventDelete {
				cc.err.Store(ErrObjectDeleted)
				return
			}
		}
	}
}

func (cc *watchContext[T]) Deadline() (time.Time, bool) {
	return cc.base.Deadline()
}

func (cc *watchContext[T]) Done() <-chan struct{} {
	return cc.done
}

func (cc *watchContext[T]) Err() error {
	return cc.err.Load()
}

func (cc *watchContext[T]) Value(key any) any {
	return cc.base.Value(key)
}

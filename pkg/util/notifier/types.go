// Package for abstracting agent downstream updates and notifying upstream
package notifier

import (
	"context"
	"errors"
)

type Clonable[T any] interface {
	Clone() T
}

type Finder[T Clonable[T]] interface {
	Find(ctx context.Context) ([]T, error)
}

type UpdateNotifier[T any] interface {
	// Returns a channel that will receive a list of update groups whenever
	// any rules are added, removed, or updated. The channel has a small buffer
	// and will initially contain the latest update group list.
	//
	// If the context is canceled, the channel will be closed. Additionally, if
	// the channel's buffer is full, any updates will be dropped.
	NotifyC(ctx context.Context) <-chan []T

	Refresh(ctx context.Context)
}

type multiFinder[T Clonable[T]] struct {
	finders []Finder[T]
}

func (m *multiFinder[T]) Find(ctx context.Context) ([]T, error) {
	var out []T
	var errs []error
	for _, finder := range m.finders {
		items, err := finder.Find(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, items...)
	}
	return out, errors.Join(errs...)
}

func NewMultiFinder[T Clonable[T]](finders ...Finder[T]) Finder[T] {
	if len(finders) == 1 {
		return finders[0]
	}
	return &multiFinder[T]{
		finders: finders,
	}
}

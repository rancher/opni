/// Package for abstracting agent downstream updates and notifying upstream
package notifier

import (
	"context"
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

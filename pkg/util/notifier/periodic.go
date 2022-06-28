package notifier

import (
	"context"
	"time"
)

type periodicUpdateNotifier[T Clonable[T]] struct {
	UpdateNotifier[T]
}

func NewPeriodicUpdateNotifier[T Clonable[T]](ctx context.Context, finder Finder[T], interval time.Duration) UpdateNotifier[T] {
	notifier := &periodicUpdateNotifier[T]{
		UpdateNotifier: NewUpdateNotifier(finder),
	}
	go func() {
		t := time.NewTicker(interval)
		for {
			// this will block until there is at least one listener on the update notifier
			notifier.Refresh(ctx)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
	}()
	return notifier
}

package rules

import (
	"context"
	"time"
)

type periodicUpdateNotifier struct {
	*updateNotifier
}

func NewPeriodicUpdateNotifier(ctx context.Context, finder RuleFinder, interval time.Duration) UpdateNotifier {
	notifier := &periodicUpdateNotifier{
		updateNotifier: NewUpdateNotifier(finder),
	}
	go func() {
		t := time.NewTicker(interval)
		for {
			// this will block until there is at least one listener on the update notifier
			notifier.FetchRules(ctx)
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

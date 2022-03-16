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
		updateNotifier: newUpdateNotifier(finder),
	}
	go func() {
		timer := time.NewTimer(interval)
		for {
			notifier.fetchRules(ctx)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
	return notifier
}

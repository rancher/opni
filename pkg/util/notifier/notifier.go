package notifier

import (
	"context"
	"sync"

	"slices"

	"github.com/google/go-cmp/cmp"
)

type test struct {
}

func (t test) Clone() test {
	return t
}

func CloneList[T Clonable[T]](list []T) []T {
	c := make([]T, 0, len(list))
	for _, t := range list {
		c = append(c, t.Clone())
	}
	return c
}

type AliasNotifer[T Clonable[T]] updateNotifier[T]

type updateNotifier[T Clonable[T]] struct {
	finder         Finder[T]
	updateChannels []chan []T
	channelsMu     *sync.Mutex
	startCond      *sync.Cond

	gcQueue chan chan []T

	latest   []T
	latestMu sync.Mutex
}

var _ UpdateNotifier[test] = (*updateNotifier[test])(nil)

func NewUpdateNotifier[T Clonable[T]](finder Finder[T]) UpdateNotifier[T] {
	mu := &sync.Mutex{}
	return &updateNotifier[T]{
		finder:         finder,
		updateChannels: []chan []T{},
		channelsMu:     mu,
		startCond:      sync.NewCond(mu),
		latest:         []T{},
		gcQueue:        make(chan chan []T, 128),
	}
}

func (u *updateNotifier[T]) NotifyC(ctx context.Context) <-chan []T {
	u.channelsMu.Lock()
	defer u.channelsMu.Unlock()
	updateC := make(chan []T, 3)
	u.updateChannels = append(u.updateChannels, updateC)
	if len(u.updateChannels) == 1 {
		// If this was the first channel to be added, unlock any calls to
		// fetchRules which might be waiting.
		u.startCond.Broadcast()
	}
	go func() {
		<-ctx.Done()
		u.gcQueue <- updateC
	}()
	return updateC
}

func (u *updateNotifier[T]) gc() {
	u.channelsMu.Lock()
	defer u.channelsMu.Unlock()
	for {
		select {
		case toDelete := <-u.gcQueue:
			u.updateChannels = slices.DeleteFunc(u.updateChannels, func(uc chan []T) bool {
				return toDelete == uc
			})
		default:
			return
		}
	}
}

func (u *updateNotifier[T]) Refresh(ctx context.Context) {
	u.channelsMu.Lock()
	for len(u.updateChannels) == 0 {
		// If there are no channels yet, wait until one is added.
		u.startCond.Wait()
	}
	u.channelsMu.Unlock()
	groups, err := u.finder.Find(ctx)
	if err != nil {
		return
	}
	modified := false
	u.latestMu.Lock()
	defer u.latestMu.Unlock()
	// compare the lengths as a quick preliminary check
	if len(groups) == len(u.latest) {
		// If the lengths are the same, compare the contents of each rule
		for i := 0; i < len(groups); i++ {
			if !cmp.Equal(groups[i], u.latest[i]) {
				modified = true
				break
			}
		}
	} else {
		modified = true
	}
	if !modified {
		return
	}
	u.latest = groups

	u.gc()
	u.channelsMu.Lock()
	cloned := CloneList(u.latest)
	for _, c := range u.updateChannels {
		select {
		case c <- cloned:
			cloned = CloneList(u.latest)
		default:
		}
	}
	u.channelsMu.Unlock()
}

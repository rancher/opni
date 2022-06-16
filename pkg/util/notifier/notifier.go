package notifier

import (
	"context"
	"sync"

	"github.com/google/go-cmp/cmp"
	"github.com/rancher/opni/pkg/util/waitctx"
	"golang.org/x/exp/slices"
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

	latest   []T
	latestMu sync.Mutex
}

var _ UpdateNotifier[test] = (*updateNotifier[test])(nil)

func NewUpdateNotifier[T Clonable[T]](finder Finder[T]) *updateNotifier[T] {
	mu := &sync.Mutex{}
	return &updateNotifier[T]{
		finder:         finder,
		updateChannels: []chan []T{},
		channelsMu:     mu,
		startCond:      sync.NewCond(mu),
		latest:         []T{},
	}
}

func (u *updateNotifier[T]) NotifyC(ctx waitctx.PermissiveContext) <-chan []T {
	u.channelsMu.Lock()
	defer u.channelsMu.Unlock()
	updateC := make(chan []T, 3)
	u.updateChannels = append(u.updateChannels, updateC)
	if len(u.updateChannels) == 1 {
		// If this was the first channel to be added, unlock any calls to
		// fetchRules which might be waiting.
		u.startCond.Broadcast()
	}
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		u.channelsMu.Lock()
		defer u.channelsMu.Unlock()
		// Remove the channel from the list
		for i, c := range u.updateChannels {
			if c == updateC {
				u.updateChannels = slices.Delete(u.updateChannels, i, i+1)
				break
			}
		}
	})
	return updateC
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

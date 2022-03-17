package rules

import (
	"context"
	"sync"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/model/rulefmt"
)

type updateNotifier struct {
	finder         RuleFinder
	updateChannels []chan []rulefmt.RuleGroup
	channelsMu     *sync.Mutex
	startCond      *sync.Cond

	latest   []rulefmt.RuleGroup
	latestMu sync.Mutex
}

func newUpdateNotifier(finder RuleFinder) *updateNotifier {
	mu := &sync.Mutex{}
	return &updateNotifier{
		finder:         finder,
		updateChannels: []chan []rulefmt.RuleGroup{},
		startCond:      sync.NewCond(mu),
		channelsMu:     mu,
		latest:         []rulefmt.RuleGroup{},
	}
}

func (u *updateNotifier) NotifyC(ctx context.Context) <-chan []rulefmt.RuleGroup {
	u.channelsMu.Lock()
	defer u.channelsMu.Unlock()
	updateC := make(chan []rulefmt.RuleGroup, 3)
	u.updateChannels = append(u.updateChannels, updateC)
	if len(u.updateChannels) == 1 {
		// If this was the first channel to be added, unlock any calls to
		// fetchRules which might be waiting.
		u.startCond.Broadcast()
	}
	go func() {
		<-ctx.Done()
		u.channelsMu.Lock()
		defer u.channelsMu.Unlock()
		// Remove the channel from the list
		for i, c := range u.updateChannels {
			if c == updateC {
				u.updateChannels = append(u.updateChannels[:i], u.updateChannels[i+1:]...)
				break
			}
		}
	}()
	return updateC
}

func (u *updateNotifier) fetchRules(ctx context.Context) {
	u.channelsMu.Lock()
	for len(u.updateChannels) == 0 {
		// If there are no channels yet, wait until one is added.
		u.startCond.Wait()
	}
	u.channelsMu.Unlock()
	groups, err := u.finder.FindGroups(context.Background())
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
	cloned := CloneRuleGroupList(u.latest)
	for _, c := range u.updateChannels {
		select {
		case c <- cloned:
			cloned = CloneRuleGroupList(u.latest)
		default:
		}
	}
	u.channelsMu.Unlock()
}

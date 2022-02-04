package test

import (
	"context"
	"sort"
	"sync"
	"time"
)

type Lease struct {
	ID         int64
	Expiration time.Time
	TokenID    string
}

type LeaseStore struct {
	leases       []*Lease
	counter      int64
	mu           sync.Mutex
	refresh      chan struct{}
	leaseExpired chan string
}

func NewLeaseStore(ctx context.Context) *LeaseStore {
	ls := &LeaseStore{
		leases:       []*Lease{},
		counter:      0,
		refresh:      make(chan struct{}),
		leaseExpired: make(chan string, 256),
	}
	go ls.run(ctx)
	return ls
}

func (ls *LeaseStore) New(tokenID string, ttl time.Duration) *Lease {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.counter++
	l := &Lease{
		ID:         ls.counter,
		Expiration: time.Now().Add(ttl),
		TokenID:    tokenID,
	}
	ls.leases = append(ls.leases, l)
	sort.Slice(ls.leases, func(i, j int) bool {
		return ls.leases[i].Expiration.Before(ls.leases[j].Expiration)
	})
	ls.refresh <- struct{}{}
	return l
}

func (ls *LeaseStore) LeaseExpired() <-chan string {
	return ls.leaseExpired
}

func (ls *LeaseStore) run(ctx context.Context) {
	for {
		ls.mu.Lock()
		count := len(ls.leases)
		ls.mu.Unlock()
		if count == 0 {
			select {
			case <-ctx.Done():
				return
			case <-ls.refresh:
				continue
			}
		}
		ls.mu.Lock()
		firstExpiration := ls.leases[0].Expiration
		ls.mu.Unlock()

		timer := time.NewTimer(time.Until(firstExpiration))
		select {
		case <-ctx.Done():
			return
		case <-ls.refresh:
		case <-time.After(time.Until(firstExpiration)):
			ls.expireFirst()
		}
		if !timer.Stop() {
			<-timer.C
		}
	}
}

func (ls *LeaseStore) expireFirst() {
	ls.mu.Lock()
	l := ls.leases[0]
	ls.leases = ls.leases[1:]
	ls.leaseExpired <- l.TokenID
	ls.mu.Unlock()
}

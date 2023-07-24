package lock

import (
	"sync"
)

type LockAggregation struct {
	tokenAggregation uint64
	largestToken     uint64
}

type LockPool struct {
	mu    sync.Mutex
	locks map[string]LockAggregation
}

func NewLockPool() *LockPool {
	return &LockPool{
		locks: map[string]LockAggregation{},
		mu:    sync.Mutex{},
	}
}

func (l *LockPool) AddLock(key string, token uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.locks[key]; !ok {
		l.locks[key] = LockAggregation{
			tokenAggregation: token,
			largestToken:     token,
		}
	} else {
		agg := LockAggregation{
			tokenAggregation: l.locks[key].tokenAggregation ^ token,
		}
		if l.locks[key].largestToken < token {
			agg.largestToken = token
		}
		l.locks[key] = agg
	}
}

func (l *LockPool) RemoveLock(key string, token uint64) (empty bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.locks[key]; ok {
		agg := LockAggregation{
			tokenAggregation: l.locks[key].tokenAggregation ^ token,
			largestToken:     l.locks[key].largestToken,
		}
		if agg.tokenAggregation == 0 {
			delete(l.locks, key)
			return true
		}
		l.locks[key] = agg
		return false
	}
	return true
}

func (l *LockPool) Holds(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.locks[key]; ok {
		return true
	}
	return false
}

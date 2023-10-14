package inmemory

import (
	"sync"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
)

type LockManager struct {
	lockMap util.LockMap[string, *sync.Mutex]
}

// NewLockManager returns a new in-memory lock manager.
func NewLockManager() *LockManager {
	return &LockManager{
		lockMap: util.NewLockMap[string, *sync.Mutex](),
	}
}

// NewLockManagerBroker returns a new in-memory lock manager broker.
func NewLockManagerBroker() *LockManagerBroker {
	return &LockManagerBroker{
		lockManagers: map[string]*LockManager{},
	}
}

type LockManagerBroker struct {
	mu           sync.Mutex
	lockManagers map[string]*LockManager
}

// LockManager implements storage.LockManagerBroker.
func (lmb *LockManagerBroker) LockManager(name string) storage.LockManager {
	lmb.mu.Lock()
	defer lmb.mu.Unlock()
	if _, ok := lmb.lockManagers[name]; !ok {
		lmb.lockManagers[name] = NewLockManager()
	}
	return lmb.lockManagers[name]
}

func (lm *LockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	return &Lock{
		mu: lm.lockMap.Get(key),
	}
}

type Lock struct {
	key string
	mu  *sync.Mutex
}

// Key implements storage.Lock.
func (l *Lock) Key() string {
	return l.key
}

// Lock implements storage.Lock.
func (l *Lock) Lock() error {
	l.mu.Lock()
	return nil
}

// TryLock implements storage.Lock.
func (l *Lock) TryLock() (bool, error) {
	ok := l.mu.TryLock()
	return ok, nil
}

// Unlock implements storage.Lock.
func (l *Lock) Unlock() error {
	l.mu.Unlock()
	return nil
}

var _ storage.Lock = (*Lock)(nil)

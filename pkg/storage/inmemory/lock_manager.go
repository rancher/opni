package inmemory

import (
	"context"
	"os"
	"path"
	"sync"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	golock "github.com/viney-shih/go-lock"
	"go.uber.org/zap"
)

type InMemoryLockManager struct {
	ctx      context.Context
	basePath string
	lg       *zap.SugaredLogger

	lockSubmitMu sync.RWMutex
	lockSubmit   map[string]*golock.ChanMutex
}

func (i *InMemoryLockManager) addSubmitter(key string) *golock.ChanMutex {
	i.lockSubmitMu.Lock()
	defer i.lockSubmitMu.Unlock()
	if _, ok := i.lockSubmit[key]; ok {
		panic("panic for now")
	}
	m := golock.NewChanMutex()
	i.lockSubmit[key] = m
	return m
	// submitter := make(chan func() error, 1)
	// i.lockSubmit[key] = submitter
	// go func() {
	// 	for {
	// 		f, ok := <-submitter
	// 		if !ok {
	// 			i.lg.Infof("submitter closed", "key", key)
	// 			return
	// 		}
	// 		if err := f(); err != nil {
	// 			i.lg.Error("error submitting lock action", "err", err)
	// 		}
	// 	}
	// }()
	// return submitter
}

// TODO : figure out when it is safe to call this
func (i *InMemoryLockManager) removeSubmitter(key string) {
	i.lockSubmitMu.Lock()
	defer i.lockSubmitMu.Unlock()
	if _, ok := i.lockSubmit[key]; ok {
		i.lockSubmit[key].Unlock()
		delete(i.lockSubmit, key)
	}
}

func (i *InMemoryLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	i.lockSubmitMu.RLock()
	submitter, ok := i.lockSubmit[key]
	i.lockSubmitMu.RUnlock()
	if !ok {
		submitter = i.addSubmitter(key)

	}
	l := NewLock(i.ctx, path.Join(i.basePath, key), i.lg, submitter, opts...)

	return l
}

func NewLockManager(ctx context.Context, basePath string) *InMemoryLockManager {
	if !path.IsAbs(basePath) {
		panic("base path must be absolute")
	}

	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		if err := os.MkdirAll(basePath, 0755); err != nil {
			panic(err)
		}
	} else if err != nil {
		panic(err)
	}

	return &InMemoryLockManager{
		ctx:        ctx,
		basePath:   basePath,
		lg:         logger.NewPluginLogger().Named("inmem-lock-manager"),
		lockSubmit: map[string]*golock.ChanMutex{},
	}
}

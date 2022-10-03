package util

import (
	"context"
	"sync"
)

type Initializer struct {
	setupCondOnce sync.Once
	initCond      *sync.Cond
	initialized   bool
}

func (i *Initializer) InitOnce(f func()) {
	i.checkInitCond()
	i.initCond.L.Lock()
	defer i.initCond.L.Unlock()
	if i.initialized {
		return
	}
	f()
	i.initialized = true
	i.initCond.Broadcast()
}

func (i *Initializer) Initialized() bool {
	i.checkInitCond()
	i.initCond.L.Lock()
	defer i.initCond.L.Unlock()
	return i.initialized
}

func (i *Initializer) WaitForInit() {
	i.checkInitCond()
	i.initCond.L.Lock()
	for !i.initialized {
		i.initCond.Wait()
	}
	i.initCond.L.Unlock()
}

func (i *Initializer) WaitForInitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		i.WaitForInit()
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *Initializer) checkInitCond() {
	i.setupCondOnce.Do(func() {
		i.initCond = sync.NewCond(&sync.Mutex{})
	})
}

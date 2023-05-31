package util

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/opensearch/opensearch"
)

type AsyncOpensearchClient struct {
	*opensearch.Client

	initCond    *sync.Cond
	initialized bool
	rw          sync.RWMutex
}

type setterFunc func(context.Context) *opensearch.Client

func NewAsyncOpensearchClient() *AsyncOpensearchClient {
	return &AsyncOpensearchClient{
		initCond: sync.NewCond(&sync.Mutex{}),
	}
}

func (c *AsyncOpensearchClient) WaitForInit() {
	c.initCond.L.Lock()
	for !c.initialized {
		c.initCond.Wait()
	}
	c.initCond.L.Unlock()
}

func (c *AsyncOpensearchClient) WaitForInitWithTimeout(timeout time.Duration) bool {
	notifier := make(chan struct{})
	go func() {
		c.initCond.L.Lock()
		defer c.initCond.L.Unlock()
		defer close(notifier)
		for !c.initialized {
			c.initCond.Wait()
		}
	}()

	select {
	case <-notifier:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (c *AsyncOpensearchClient) SetClient(setter setterFunc) {
	c.rw.Lock()
	defer c.rw.Unlock()

	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()

	if c.initialized {
		return
	}
	c.Client = setter(context.TODO())
	c.initialized = true
	c.initCond.Broadcast()
}

func (c *AsyncOpensearchClient) UnsetClient() {
	c.rw.Lock()
	defer c.rw.Unlock()

	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	if !c.initialized {
		return
	}
	c.Client = nil
	c.initialized = false
}

func (c *AsyncOpensearchClient) Lock() {
	c.rw.RLock()
}

func (c *AsyncOpensearchClient) Unlock() {
	c.rw.RUnlock()
}

func (c *AsyncOpensearchClient) IsInitialized() bool {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.initialized
}

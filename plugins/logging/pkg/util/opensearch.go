package util

import (
	"sync"

	"github.com/rancher/opni/pkg/util/opensearch"
)

type AsyncOpensearchClient struct {
	opensearch.ExtendedClient

	initCond    *sync.Cond
	initialized bool
	rw          sync.RWMutex
}

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

func (c *AsyncOpensearchClient) SetClient(setter func() opensearch.ExtendedClient) {
	c.rw.Lock()
	defer c.rw.Unlock()

	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()

	if c.initialized {
		return
	}
	c.ExtendedClient = setter()
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
	c.initialized = false
}

func (c *AsyncOpensearchClient) Lock() {
	c.rw.RLock()
}

func (c *AsyncOpensearchClient) Unlock() {
	c.rw.RUnlock()
}

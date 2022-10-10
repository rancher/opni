package util

import (
	"sync"

	osclient "github.com/opensearch-project/opensearch-go"
)

type AsyncOpensearchClient struct {
	*osclient.Client

	setupCondOnce sync.Once
	initCond      *sync.Cond
	initialized   bool
}

func (c *AsyncOpensearchClient) WaitForInit() {
	c.checkInitCond()
	c.initCond.L.Lock()
	for !c.initialized {
		c.initCond.Wait()
	}
	c.initCond.L.Unlock()
}

func (c *AsyncOpensearchClient) SetClient(setter func() *osclient.Client) {
	c.checkInitCond()
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	if c.initialized {
		return
	}
	c.Client = setter()
	c.initialized = true
	c.initCond.Broadcast()
}

func (c *AsyncOpensearchClient) UnsetClient() {
	c.checkInitCond()
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	if !c.initialized {
		return
	}
	c.initialized = false
}

func (c *AsyncOpensearchClient) Lock() {
	c.checkInitCond()
	c.initCond.L.Lock()
}

func (c *AsyncOpensearchClient) Unlock() {
	c.checkInitCond()
	c.initCond.L.Unlock()
}

func (c *AsyncOpensearchClient) checkInitCond() {
	c.setupCondOnce.Do(func() {
		c.initCond = sync.NewCond(&sync.Mutex{})
	})
}

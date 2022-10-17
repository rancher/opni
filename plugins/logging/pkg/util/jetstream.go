package util

import (
	"sync"

	"github.com/nats-io/nats.go"
)

type AsyncJetStreamClient struct {
	nats.KeyValue

	initCond    *sync.Cond
	initialized bool
}

func NewAsyncJetStreamClient() *AsyncJetStreamClient {
	return &AsyncJetStreamClient{
		initCond: sync.NewCond(&sync.Mutex{}),
	}
}

func (c *AsyncJetStreamClient) WaitForInit() {
	c.initCond.L.Lock()
	for !c.initialized {
		c.initCond.Wait()
	}
	c.initCond.L.Unlock()
}

func (c *AsyncJetStreamClient) SetClient(setter func() nats.KeyValue) {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()

	if c.initialized {
		return
	}
	c.KeyValue = setter()
	c.initialized = true
	c.initCond.Broadcast()
}

func (c *AsyncOpensearchClient) IsInitilized() bool {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.initialized
}

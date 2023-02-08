package util

import "sync"

type AsyncClient[T any] struct {
	Client      T
	initCond    *sync.Cond
	initialized bool
}

func NewAsyncClient[T any]() *AsyncClient[T] {
	return &AsyncClient[T]{
		initCond: sync.NewCond(&sync.Mutex{}),
	}
}

func (c *AsyncClient[T]) WaitForInit() {
	c.initCond.L.Lock()
	for !c.initialized {
		c.initCond.Wait()
	}
	c.initCond.L.Unlock()
}

func (c *AsyncClient[T]) IsInitialized() bool {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	return c.initialized
}

func (c *AsyncClient[T]) SetClient(setter func() T, alwaysSet bool) {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()

	if c.initialized {
		if !alwaysSet {
			return
		}
		c.Client = setter()
		return
	}
	c.Client = setter()
	c.initialized = true
	c.initCond.Broadcast()
}

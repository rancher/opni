package util

import "sync"

type AsyncClient[T any] struct {
	Client   T
	initCond *sync.Cond
	set      bool
}

func NewAsyncClient[T any]() *AsyncClient[T] {
	return &AsyncClient[T]{
		initCond: sync.NewCond(&sync.Mutex{}),
	}
}

func (c *AsyncClient[T]) WaitForInit() {
	c.initCond.L.Lock()
	for !c.set {
		c.initCond.Wait()
	}
	c.initCond.L.Unlock()
}

func (c *AsyncClient[T]) IsSet() bool {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	return c.set
}

// BackgroundInitClient will intialize the client only if it is curently unset. This can be called multiple times.
func (c *AsyncClient[T]) BackgroundInitClient(setter func() T) {
	go c.doBackgroundInit(setter)
}

func (c *AsyncClient[T]) doBackgroundInit(setter func() T) {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()

	if c.set {
		return
	}
	c.Client = setter()
	c.set = true
	c.initCond.Broadcast()
}

// SetClient will always update the client regardless of its previous confition.
func (c *AsyncClient[T]) SetClient(client T) {
	c.initCond.L.Lock()
	defer c.initCond.L.Unlock()
	c.Client = client
	shouldBroadcast := !c.set
	c.set = true
	if shouldBroadcast {
		c.initCond.Broadcast()
	}
}

package health

import (
	"context"
)

// Buffer implements a HealthStatusUpdateReader and HealthStatusUpdateWriter
// using buffered channels.
type Buffer struct {
	healthC chan HealthUpdate
	statusC chan StatusUpdate
}

func NewBuffer() *Buffer {
	return &Buffer{
		healthC: make(chan HealthUpdate, 256),
		statusC: make(chan StatusUpdate, 256),
	}
}

func AsBuffer(healthC chan HealthUpdate, statusC chan StatusUpdate) *Buffer {
	return &Buffer{
		healthC: healthC,
		statusC: statusC,
	}
}

func (c *Buffer) Close() {
	close(c.healthC)
	close(c.statusC)
}

func (c *Buffer) HealthC() <-chan HealthUpdate {
	return c.healthC
}

func (c *Buffer) StatusC() <-chan StatusUpdate {
	return c.statusC
}

func (c *Buffer) HealthWriterC() chan<- HealthUpdate {
	return c.healthC
}

func (c *Buffer) StatusWriterC() chan<- StatusUpdate {
	return c.statusC
}

func Copy(ctx context.Context, dst HealthStatusUpdateWriter, src HealthStatusUpdateReader) {
	srcHealth := src.HealthC()
	srcStatus := src.StatusC()
	dstHealth := dst.HealthWriterC()
	dstStatus := dst.StatusWriterC()

	for {
		select {
		case <-ctx.Done():
			return
		case health, ok := <-srcHealth:
			if ok {
				dstHealth <- health
			} else {
				for {
					select {
					case <-ctx.Done():
						return
					case status, ok := <-srcStatus:
						if ok {
							dstStatus <- status
						} else {
							return
						}
					}
				}
			}
		case status, ok := <-srcStatus:
			if ok {
				dstStatus <- status
			} else {
				for {
					select {
					case <-ctx.Done():
						return
					case health, ok := <-srcHealth:
						if ok {
							dstHealth <- health
						} else {
							return
						}
					}
				}
			}
		}
	}
}

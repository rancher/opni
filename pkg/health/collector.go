package health

import "context"

// Collector implements HealthStatusUpdater by aggregating the incoming health
// and status updates from multiple source HealthStatusUpdater instances.
type Collector struct {
	healthC chan HealthUpdate
	statusC chan StatusUpdate
}

func NewCollector() *Collector {
	return &Collector{
		healthC: make(chan HealthUpdate, 256),
		statusC: make(chan StatusUpdate, 256),
	}
}

func (c *Collector) HealthC() <-chan HealthUpdate {
	return c.healthC
}

func (c *Collector) StatusC() <-chan StatusUpdate {
	return c.statusC
}

// Collect watches for incoming health and status updates from the updater and
// forwards them the collector's health and status channels. This method blocks
// until the context is canceled.
func (c *Collector) Collect(ctx context.Context, updater HealthStatusUpdateReader) {
	healthC := updater.HealthC()
	statusC := updater.StatusC()
	for {
		select {
		case <-ctx.Done():
			return
		case health := <-healthC:
			c.healthC <- health
		case status := <-statusC:
			c.statusC <- status
		}
	}
}

package patch

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

type CacheMetrics struct {
	CacheMisses    int64
	CacheHits      int64
	PluginCount    int64
	PatchCount     int64
	TotalSizeBytes int64
}

type CacheMetricsTracker struct {
	metrics CacheMetrics

	promCacheMisses    prometheus.Counter
	promCacheHits      prometheus.Counter
	promPluginCount    prometheus.Gauge
	promPatchCount     prometheus.Gauge
	promTotalSizeBytes prometheus.Gauge
}

func NewCacheMetricsTracker(constLabels map[string]string) CacheMetricsTracker {
	return CacheMetricsTracker{
		promCacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "cache_misses_total",
			ConstLabels: constLabels,
			Help:        "Total number of patch requests that were not found in the cache",
		}),
		promCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "cache_hits_total",
			ConstLabels: constLabels,
			Help:        "Total number of patch requests that were found in the cache",
		}),
		promPluginCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "plugin_count",
			ConstLabels: constLabels,
			Help:        "Total number of plugin revisions in the cache",
		}),
		promPatchCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "patch_count",
			ConstLabels: constLabels,
			Help:        "Total number of patches in the cache",
		}),
		promTotalSizeBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "total_size_bytes",
			ConstLabels: constLabels,
			Help:        "Total size of the cache in bytes",
		}),
	}
}

func (c *CacheMetricsTracker) CacheMiss() {
	atomic.AddInt64(&c.metrics.CacheMisses, 1)
	c.promCacheMisses.Inc()
}

func (c *CacheMetricsTracker) CacheHit() {
	atomic.AddInt64(&c.metrics.CacheHits, 1)
	c.promCacheHits.Inc()
}

func (c *CacheMetricsTracker) SetPluginCount(count int64) {
	atomic.StoreInt64(&c.metrics.PluginCount, count)
	c.promPluginCount.Set(float64(count))
}

func (c *CacheMetricsTracker) AddToPluginCount(value int64) {
	atomic.AddInt64(&c.metrics.PluginCount, value)
	c.promPluginCount.Add(float64(value))
}

func (c *CacheMetricsTracker) SetPatchCount(count int64) {
	atomic.StoreInt64(&c.metrics.PatchCount, count)
	c.promPatchCount.Set(float64(count))
}
func (c *CacheMetricsTracker) AddToPatchCount(value int64) {
	atomic.AddInt64(&c.metrics.PatchCount, value)
	c.promPatchCount.Add(float64(value))
}

func (c *CacheMetricsTracker) SetTotalSizeBytes(size int64) {
	atomic.StoreInt64(&c.metrics.TotalSizeBytes, size)
	c.promTotalSizeBytes.Set(float64(size))
}

func (c *CacheMetricsTracker) AddToTotalSizeBytes(value int64) {
	atomic.AddInt64(&c.metrics.TotalSizeBytes, value)
	c.promTotalSizeBytes.Add(float64(value))
}

func (c *CacheMetricsTracker) MetricsCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		c.promCacheMisses,
		c.promCacheHits,
		c.promPluginCount,
		c.promPatchCount,
		c.promTotalSizeBytes,
	}
}

func (c *CacheMetricsTracker) MetricsSnapshot() CacheMetrics {
	return CacheMetrics{
		CacheMisses:    atomic.LoadInt64(&c.metrics.CacheMisses),
		CacheHits:      atomic.LoadInt64(&c.metrics.CacheHits),
		PluginCount:    atomic.LoadInt64(&c.metrics.PluginCount),
		PatchCount:     atomic.LoadInt64(&c.metrics.PatchCount),
		TotalSizeBytes: atomic.LoadInt64(&c.metrics.TotalSizeBytes),
	}
}

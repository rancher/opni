package patch

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

const oldDigestLabel = "old_digest"
const newDigestLabel = "new_digest"
const digestLabel = "digest"

type CacheMetrics struct {
	CacheMisses           int64
	CacheHits             int64
	PluginCount           int64
	PatchCount            int64
	TotalSizeBytes        int64
	PatchCalcSecondsTotal int64
	PatchCalcCount        int64
}

type CacheMetricsTracker struct {
	metrics CacheMetrics

	promCacheMisses    *prometheus.CounterVec
	promCacheHits      *prometheus.CounterVec
	promPluginCount    prometheus.Gauge
	promPatchCount     prometheus.Gauge
	promTotalSizeBytes *prometheus.GaugeVec

	promPatchCalcNanoSecsTotal *prometheus.CounterVec
	promPatchCalcCount         *prometheus.CounterVec
}

func NewCacheMetricsTracker(constLabels map[string]string) CacheMetricsTracker {
	return CacheMetricsTracker{
		promCacheMisses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "cache_misses_total",
			ConstLabels: constLabels,
			Help:        "Total number of patch requests that were not found in the cache",
		}, []string{oldDigestLabel, newDigestLabel}),
		promCacheHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "cache_hits_total",
			ConstLabels: constLabels,
			Help:        "Total number of patch requests that were found in the cache",
		}, []string{oldDigestLabel, newDigestLabel}),
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
		promTotalSizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "total_size_bytes",
			ConstLabels: constLabels,
			Help:        "Total size of the cache in bytes",
		}, []string{digestLabel}),
		promPatchCalcNanoSecsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "calc_nano_seconds_total",
			ConstLabels: constLabels,
			Help:        "Total time spent calculating patches in nanoseconds",
		}, []string{oldDigestLabel, newDigestLabel}),
		promPatchCalcCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   "opni",
			Subsystem:   "patch",
			Name:        "calc_count_total",
			ConstLabels: constLabels,
			Help:        "Total number of patch calculations requested",
		}, []string{oldDigestLabel, newDigestLabel}),
	}
}

func (c *CacheMetricsTracker) CacheMiss(oldDigest, newDigest string) {
	atomic.AddInt64(&c.metrics.CacheMisses, 1)
	c.promCacheMisses.WithLabelValues(oldDigest, newDigest).Inc()
}

func (c *CacheMetricsTracker) CacheHit(oldDigest, newDigest string) {
	atomic.AddInt64(&c.metrics.CacheHits, 1)
	c.promCacheHits.WithLabelValues(oldDigest, newDigest).Inc()
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

func (c *CacheMetricsTracker) SetTotalSizeBytes(digest string, size int64) {
	atomic.StoreInt64(&c.metrics.TotalSizeBytes, size)
	c.promTotalSizeBytes.WithLabelValues(digest).Set(float64(size))
}

func (c *CacheMetricsTracker) AddToTotalSizeBytes(digest string, value int64) {
	atomic.AddInt64(&c.metrics.TotalSizeBytes, value)
	c.promTotalSizeBytes.WithLabelValues(digest).Add(float64(value))
}

func (c *CacheMetricsTracker) IncPatchCalcSecsTotal(oldDigest, newDigest string, value float64) {
	atomic.AddInt64(&c.metrics.PatchCalcSecondsTotal, int64(value))
	c.promPatchCalcNanoSecsTotal.WithLabelValues(oldDigest, newDigest).Add(value)
}

func (c *CacheMetricsTracker) IncPatchCalcCount(oldDigest, newDigest string) {
	atomic.AddInt64(&c.metrics.PatchCalcCount, 1)
	c.promPatchCalcCount.WithLabelValues(oldDigest, newDigest).Inc()
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

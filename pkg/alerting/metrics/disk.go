package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type DiskBytesOptions struct {
	cluster corev1.Cluster `metric:"node_disk_.*_bytes_total"`

	ioType       string `jobExtractor:"node_disk_.*_bytes_total"`
	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	forDuration  time.Duration
}

// Implements MetricOpts interface
func (d *DiskBytesOptions) MetricOptions() {}

type DiskTimeOptions struct {
	cluster      corev1.Cluster `metric:"node_disk_.*_time_seconds_total"`
	ioType       string         `jobExtractor:"node_disk_.*_time_seconds_total"`
	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	forDuration  time.Duration
}

// Implements MetricOpts interface
func (d *DiskTimeOptions) MetricOptions() {}

type DiskOperationsOptions struct {
	cluster      corev1.Cluster `metric:"node_disk_.*_completed_total"`
	ioType       string         `jobExtractor:"node_disk_.*_completed_total"`
	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	forDuration  time.Duration
}

// Implements MEtricOpts interface
func (d *DiskOperationsOptions) MetricOptions() {}

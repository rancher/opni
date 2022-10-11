package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type MemoryRuleOptions struct {
	cluster corev1.Cluster `metric:"node_memory_.*"`

	types  string `label:"type", metric:"node_memory_MemTotal_bytes"`
	node   string `label:"instance", metric:"node_memory_MemTotal_bytes"`
	device string `label:"device", metric:"node_memory_MemTotal_bytes"`

	compOperator ComparisonOperator
	percentValue float64 `range:[0,100]`
	forDuration  time.Duration
}

func (m *MemoryRuleOptions) MetricOptions() {}

package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func NewCpuRule() (*AlertingRule, error) {
	// TODO
	return nil, nil
}

type CpuRuleOptions struct {
	Cluster *corev1.Cluster `metric:"node_cpu_seconds_total"`

	Node      []string `label:"instance" metric:"node_cpu_seconds_total"`
	UsageMode string   `label:"mode" metric:"node_cpu_seconds_total"`
	Cpu       []string `label:"cpu" metric:"node_cpu_seconds_total"`

	CompOperator ComparisonOperator
	Target       float64 `range:"[0,100]"`
	ForDuration  time.Duration
	// Interval     prometheus.Duration
}

// Implements MetricOpts interface
func (c *CpuRuleOptions) MetricOptions() {}

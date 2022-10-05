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
	cluster corev1.Cluster `metric:"node_cpu_seconds_total"`

	node       string `label:"instance", metric:"node_cpu_seconds_total"`
	usageModes string `label:"mode, metric:"node_cpu_seconds_total"`
	cpus       string `label:"cpu", metric:"node_cpu_seconds_total"`

	compOperator ComparisonOperator
	target       float64 `range:[0,100]`
	forDuration  time.Duration
}

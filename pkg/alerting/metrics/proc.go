package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type ProcessesRuleOptions struct {
	cluster corev1.Cluster `metric:"node_procs_.*"`

	processType  string `jobExtractor:"node_procs_.*"`
	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	forDuration  time.Duration
}

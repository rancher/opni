package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type NetworkBytesOptions struct {
	cluster corev1.Cluster `metric:"node_network_.*_bytes_total"`

	// should be one of receive/transmit
	transmitType string `jobExtractor:"node_network_.*_bytes_total"`
	// memory modes
	memoryMode string `label:"TODO", metric:"node_network_.*_bytes_total"`
	node       string `label:"instance", metric:"node_network_.*_bytes_total"`

	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	// for duration
	forDuration time.Duration
	// node
}

func NewNetworkBytesRule() (*AlertingRule, error) {
	// TODO
	return nil, nil
}

type NetworkErrorsOptions struct{}

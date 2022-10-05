package metrics

import (
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type FilesystemUsageOptions struct {
	cluster corev1.Cluster `metric:"node_filesystem_avail_bytes`

	mountPoint string `label:"mountpoint", metric:"node_filesystem_avail_bytes"`
	device     string `label:"device",metric:"node_filesystem_avail_bytes"`

	compOperator ComparisonOperator
	percentValue float64 `range:[0,100]`
	forDuration  time.Duration
}

// `node_filefd_allocated` is the number of file descriptors allocated.
type FilesystemOpenFiledescriptorRuleOptions struct {
	cluster corev1.Cluster `metric:"node_filefd_allocated"`

	compOperator ComparisonOperator
	target       int64 `range:[0,inf]`
	forDuration  time.Duration
}

/// Auto discovery uses prometheus' kube server agent
/// for pod creation/deletion
/// - https://github.com/prometheus/prometheus/blob/main/documentation/examples/prometheus-kubernetes.yml
///	- https://prometheus.io/docs/prometheus/latest/configuration/configuration/#auto_discovery
/// - https://medium.com/kubernetes-tutorials/monitoring-your-kubernetes-deployments-with-prometheus-5665eda54045
/// - https://github.com/kubernetes/kube-state-metrics/blob/master/docs/pod-metrics.md

/// Use prometheus to discover pods and group them by their metadata:ownerReferences
package prometheus

import (
	"context"

	discovery "github.com/rancher/opni/pkg/discovery/v1alpha"
)

type PrometheusService struct {
	discovery.Service
}

func (p *PrometheusService) Discover(ctx context.Context) error {
	return p.Service.Discover(ctx)
}

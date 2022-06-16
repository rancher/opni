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
	"fmt"

	"github.com/rancher/opni/pkg/discovery"
)

type PrometheusService struct {
	clusterId   string
	serviceName string
	serviceId   string
	metadata    string
}

type PrometheusServiceFinder struct {
	appliedRules []discovery.DiscoveryRule
}

func NewPrometheusServiceFinder(drules ...discovery.DiscoveryRule) (*PrometheusServiceFinder, error) {
	return &PrometheusServiceFinder{
		appliedRules: drules,
	}, nil
}

func (p *PrometheusService) Discover(ctx context.Context, drules ...discovery.DiscoveryRule) error {
	for _, discover_func := range drules {
		if _, err := discover_func(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (p *PrometheusService) GetClusterId() string {
	return p.clusterId
}

func (p *PrometheusService) GetServiceName() string {
	return p.serviceName
}

func (p *PrometheusService) GetServiceId() string {
	return p.serviceId
}

func QueryAllPromEndpoints(ctx context.Context) (map[string]string, error) {
	res := map[string]string{}
	query_str := "/api/v1/label/__name__/values"
	metadata_query_str := "/api/v1/metadata"

	fmt.Print(query_str, metadata_query_str)
	return res, nil
}

/// Test only : make sure the stream works, regardless of discovery implementation
func FakePrometheusDiscover(ctx context.Context) ([]discovery.Service, error) {
	return []discovery.Service{
		&PrometheusService{
			clusterId:   "cluster1",
			serviceName: "service1",
			serviceId:   "service1",
		},
		&PrometheusService{
			clusterId:   "cluster2",
			serviceName: "service2",
			serviceId:   "service2",
		},
		&PrometheusService{
			clusterId:   "cluster3",
			serviceName: "service3",
			serviceId:   "service3",
		},
	}, nil
}

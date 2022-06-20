// Find Service in Prometheus, must implement Find
package prometheus

import (
	"context"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/discovery"
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusServiceFinder struct {
	PrometheusServiceFinderOptions
	k8sClient client.Client
}

type PrometheusServiceFinderOptions struct {
	logger *zap.SugaredLogger
}

type PrometheusServiceFinderOption func(*PrometheusServiceFinderOptions)

func (o *PrometheusServiceFinderOptions) apply(opts ...PrometheusServiceFinderOption) {
	for _, op := range opts {
		op(o)
	}
}

func NewPrometheusServiceFinder(k8sClient client.Client, opts ...PrometheusServiceFinderOption) *PrometheusServiceFinder {
	options := PrometheusServiceFinderOptions{
		logger: logger.New().Named("sdiscovery"),
	}
	options.apply(opts...)
	return &PrometheusServiceFinder{
		PrometheusServiceFinderOptions: options,
		k8sClient:                      k8sClient,
	}
}

func (f *PrometheusServiceFinder) Find(ctx context.Context) ([]discovery.Service, error) {
	res := make([]discovery.Service, 0)
	kubeObjectsServices, err := f.findDiscoveryKubeObject(ctx)
	if err != nil {
		return nil, err
	}
	res = append(res, kubeObjectsServices...)

	promServerServices, err := f.findDiscoveryPromServer(ctx)
	if err != nil {
		return nil, err
	}
	res = append(res, promServerServices...)

	return res, nil
}

func (f *PrometheusServiceFinder) findDiscoveryPromServer(ctx context.Context) ([]discovery.Service, error) {
	lg := f.logger
	res := make([]discovery.Service, 0)
	// TODO search for user defined scrape configs in fs in the prometheus operators

	lg.Debug("Finding Prometheus server pods ...")

	lg.Debug("Parsing scrape configs to group targets...")

	//TODO need to make sure there are no dupes from the operator discovery
	//TODO make a test where there could potentially be duplicates if we don't parse this correctly

	return res, nil
}

func (f *PrometheusServiceFinder) findDiscoveryKubeObject(ctx context.Context) ([]discovery.Service, error) {
	lg := f.logger
	res := make([]discovery.Service, 0)

	lg.Debug("Finding Prometheus operator ServiceMonitors ...")
	// "Service" exposed scrape targets with metrics
	promServices := &monitoringv1.ServiceMonitorList{}
	if err := f.k8sClient.List(ctx, promServices); err != nil {
		return nil, err
	}
	lg.Debugf("Found %d PrometheusServiceMonitor CRDs", len(promServices.Items))

	for _, promService := range promServices.Items {
		res = append(res, PrometheusService{
			clusterId:   promService.ObjectMeta.ClusterName,
			serviceId:   promService.Spec.JobLabel,
			serviceName: promService.ObjectMeta.Name,
			serviceType: "ServiceMonitor",
		})
	}

	// "Pod" exposed scrape targets with metrics
	promPods := &monitoringv1.PodMonitorList{}
	if err := f.k8sClient.List(ctx, promPods); err != nil {
		return nil, err
	}

	lg.Debugf("Found %d PrometheusPodMonitor CRDs", len(promServices.Items))

	for _, promPod := range promPods.Items {
		res = append(res, PrometheusService{
			clusterId:   promPod.ObjectMeta.ClusterName,
			serviceId:   promPod.Spec.JobLabel,
			serviceName: promPod.ObjectMeta.Name,
			serviceType: "PodMonitor",
		})
	}
	return res, nil

}

type PrometheusService struct {
	clusterId   string
	serviceName string
	serviceId   string
	serviceType string
}

func (p PrometheusService) GetClusterId() string {
	return p.clusterId
}

func (p PrometheusService) GetServiceName() string {
	return p.serviceName
}

func (p PrometheusService) GetServiceId() string {
	return p.serviceId
}

func (p PrometheusService) GetServiceType() string {
	return p.serviceType
}

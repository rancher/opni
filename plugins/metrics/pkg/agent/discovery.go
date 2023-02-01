package agent

import (
	"context"
	"fmt"
	apimonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/samber/lo"
	"go.uber.org/zap"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type DiscovererConfig struct {
	RESTConfig *rest.Config
	Context    context.Context
	Logger     *zap.SugaredLogger
}

type PrometheusDiscoverer struct {
	DiscovererConfig

	kubeClient kubernetes.Interface
	promClient monitoringclient.Interface
}

func NewPrometheusDiscoverer(config DiscovererConfig) (*PrometheusDiscoverer, error) {
	if config.RESTConfig == nil {
		restConfig, err := rest.InClusterConfig()

		if err != nil {
			return nil, fmt.Errorf("could not create restr config: %w", err)
		}

		config.RESTConfig = restConfig
	}

	kubeClient, err := kubernetes.NewForConfig(config.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create kubernets client; %w", err)
	}

	promClient, err := monitoringclient.NewForConfig(config.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create Prometheus monitoring client: %w", err)
	}

	return &PrometheusDiscoverer{
		DiscovererConfig: config,

		kubeClient: kubeClient,
		promClient: promClient,
	}, nil
}

func (discoverer *PrometheusDiscoverer) Discover() ([]*remoteread.DiscoveryEntry, error) {
	namespaces, err := discoverer.kubeClient.CoreV1().Namespaces().List(discoverer.Context, apimetav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not fetch list of namespaces: %w", err)
	}

	entries := make([]*remoteread.DiscoveryEntry, 0)

	for _, namespace := range namespaces.Items {
		entriesInNamespace, err := discoverer.DiscoverIn(namespace.Name)
		if err != nil {
			discoverer.Logger.Errorf("error fetching Prometheuses for namespace '%s': %w", namespace, err)
		}

		entries = append(entries, entriesInNamespace...)
	}

	return entries, nil
}

func (discoverer *PrometheusDiscoverer) DiscoverIn(namespace string) ([]*remoteread.DiscoveryEntry, error) {
	list, err := discoverer.promClient.MonitoringV1().Prometheuses(namespace).List(discoverer.Context, apimetav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could complete discovery: %w", err)
	}

	return lo.Map(list.Items, func(prometheus *apimonitoringv1.Prometheus, _ int) *remoteread.DiscoveryEntry {
		return &remoteread.DiscoveryEntry{
			Name:             prometheus.Name,
			ExternalEndpoint: prometheus.Spec.ExternalURL,
			InternalEndpoint: fmt.Sprintf("%s.%s.svc.cluster.local", prometheus.Name, prometheus.Namespace),
		}
	}), nil
}

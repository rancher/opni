package agent

import (
	"context"
	"fmt"
	apimonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
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

func (discoverer *PrometheusDiscoverer) Discover() ([]*apimonitoringv1.Prometheus, error) {
	namespaces, err := discoverer.kubeClient.CoreV1().Namespaces().List(discoverer.Context, apimetav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not fetch list of namespaces: %w", err)
	}

	prometheuses := make([]*apimonitoringv1.Prometheus, 0)

	for _, namespace := range namespaces.Items {
		p, err := discoverer.DiscoverIn(namespace.Name)
		if err != nil {
			discoverer.Logger.Errorf("error fetching Prometheuses for namespace '%s': %w", namespace, err)
		}

		prometheuses = append(prometheuses, p...)
	}

	return prometheuses, nil
}

func (discoverer *PrometheusDiscoverer) DiscoverIn(namespace string) ([]*apimonitoringv1.Prometheus, error) {
	list, err := discoverer.promClient.MonitoringV1().Prometheuses(namespace).List(discoverer.Context, apimetav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could complete discovery: %w", err)
	}

	return list.Items, nil
}

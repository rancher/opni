package drivers

import (
	"context"
	"fmt"
	monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ExternalOperatorOperatorDriverName = "external-operator"
	EnvNamespace                       = "POD_NAMESPACE"
)

type ExternalOperatorDriverOptions struct {
	k8sClient client.Client
}

type ExternalOperatorDriverOption func(options *ExternalOperatorDriverOptions)

func (o *ExternalOperatorDriverOptions) apply(opts ...ExternalOperatorDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) ExternalOperatorDriverOption {
	return func(o *ExternalOperatorDriverOptions) {
		o.k8sClient = k8sClient
	}
}

type ExternalOperatorDriver struct {
	ExternalOperatorDriverOptions

	logger    *zap.SugaredLogger
	namespace string
}

func NewExternalOperatorDriver(
	logger *zap.SugaredLogger,
	opts ...ExternalOperatorDriverOption,
) (*ExternalOperatorDriver, error) {
	options := ExternalOperatorDriverOptions{}
	options.apply(opts...)

	namespace, ok := os.LookupEnv(EnvNamespace)
	if !ok {
		return nil, fmt.Errorf("%S environment variable not set", EnvNamespace)
	}

	if options.k8sClient == nil {
		k8sClient, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create k8s client: %w", err)
		}
		options.k8sClient = k8sClient
	}

	return &ExternalOperatorDriver{
		ExternalOperatorDriverOptions: options,
		logger:                        logger,
		namespace:                     namespace,
	}, nil
}

func (*ExternalOperatorDriver) Name() string {
	return ExternalOperatorOperatorDriverName
}

func (d *ExternalOperatorDriver) DiscoverPrometheuses(ctx context.Context, namespace string) ([]*remoteread.DiscoveryEntry, error) {
	list := &monitoringcoreosv1.PrometheusList{}
	if err := d.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	return lo.Map(list.Items, func(prom *monitoringcoreosv1.Prometheus, _ int) *remoteread.DiscoveryEntry {
		return &remoteread.DiscoveryEntry{
			Name:             prom.Name,
			ClusterId:        "", // populated by the gateway
			ExternalEndpoint: prom.Spec.ExternalURL,
			InternalEndpoint: fmt.Sprintf("%s.%s.svc.cluster.local", prom.Name, prom.Namespace),
		}
	}), nil
}

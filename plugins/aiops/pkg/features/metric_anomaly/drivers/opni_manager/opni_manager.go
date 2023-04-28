package opni_manager

import (
	"context"
	"fmt"
	"os"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly/drivers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpniManagerClusterDriverOptions struct {
	K8sClient client.Client `option:"k8sClient"`
	Namespace string        `option:"namespace"`
}

type OpniManager struct {
	OpniManagerClusterDriverOptions
}

var dashboardSelector = &metav1.LabelSelector{
	MatchLabels: map[string]string{
		resources.AppNameLabel:  "grafana",
		resources.PartOfLabel:   "opni",
		resources.InstanceLabel: "opni", // TODO: this should be the name of MonitoringCluster
	},
}

func (o *OpniManager) CreateDashboard(name string, dashboardJson string) error {
	grafanaDashboard := &grafanav1alpha1.GrafanaDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: o.Namespace,
			Labels:    dashboardSelector.MatchLabels,
		},
		Spec: grafanav1alpha1.GrafanaDashboardSpec{
			Json: dashboardJson,
		},
	}
	return o.K8sClient.Create(context.Background(), grafanaDashboard)
}

func (o *OpniManager) DeleteDashboard(name string) error {
	grafanaDashboard := &grafanav1alpha1.GrafanaDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: o.Namespace,
			Labels:    dashboardSelector.MatchLabels,
		},
	}
	return o.K8sClient.Delete(context.Background(), grafanaDashboard)
}

func NewOpniManagerDashboardDriver(options OpniManagerClusterDriverOptions) (*OpniManager, error) {
	if options.K8sClient == nil {
		s := scheme.Scheme
		opnicorev1beta1.AddToScheme(s)
		grafanav1alpha1.AddToScheme(s)
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: s,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.K8sClient = c
	}
	return &OpniManager{
		OpniManagerClusterDriverOptions: options,
	}, nil
}

func init() {
	drivers.DashboardDrivers.Register("opni-manager", func(_ context.Context, opts ...driverutil.Option) (drivers.DashboardDriver, error) {
		options := OpniManagerClusterDriverOptions{
			Namespace: os.Getenv("POD_NAMESPACE"),
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}

		return NewOpniManagerDashboardDriver(options)
	})
}

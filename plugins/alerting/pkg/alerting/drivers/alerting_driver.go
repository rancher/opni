package drivers

import (
	"context"
	"os"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingManager struct {
	AlertingManagerDriverOptions
	alertops.UnsafeAlertingOpsServer
}

var _ ClusterDriver = (*AlertingManager)(nil)
var _ alertops.AlertingOpsServer = (*AlertingManager)(nil)

type AlertingManagerDriverOptions struct {
	k8sClient         client.Client
	gatewayRef        types.NamespacedName
	gatewayApiVersion string
}

type AlertingManagerDriverOption func(*AlertingManagerDriverOptions)

func (a *AlertingManagerDriverOptions) apply(opts ...AlertingManagerDriverOption) {
	for _, o := range opts {
		o(a)
	}
}

func WithK8sClient(client client.Client) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.k8sClient = client
	}
}

func WithGatewayRef(ref types.NamespacedName) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.gatewayRef = ref
	}
}

func WithGatewayApiVersion(version string) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.gatewayApiVersion = version
	}
}

func NewAlertingManagerDriver(opts ...AlertingManagerDriverOption) (*AlertingManager, error) {
	options := AlertingManagerDriverOptions{
		gatewayRef: types.NamespacedName{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Name:      os.Getenv("GATEWAY_NAME"),
		},
		gatewayApiVersion: os.Getenv("GATEWAY_API_VERSION"),
	}
	options.apply(opts...)

	return &AlertingManager{
		AlertingManagerDriverOptions: options,
	}, nil
}

func (a *AlertingManager) Name() string {
	return "alerting-mananger"
}

func (a *AlertingManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

func (a *AlertingManager) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	// TODO : implement
	return nil, nil
}

func (a *AlertingManager) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	// TODO : implement
	return nil, nil
}

func (a *AlertingManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	// TODO : implement
	return nil, nil
}

func (a *AlertingManager) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// TODO : implement
	return nil, nil
}

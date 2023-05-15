package alerting_manager

import (
	"context"
	"fmt"
	"os"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingGatewayManager struct {
	AlertingDriverOptions
	alertops.UnsafeAlertingAdminServer
}

func (a *AlertingGatewayManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

var _ drivers.ClusterDriver = (*AlertingGatewayManager)(nil)
var _ alertops.AlertingAdminServer = (*AlertingGatewayManager)(nil)

func NewAlertingGatewayManager(options AlertingDriverOptions) (*AlertingGatewayManager, error) {
	if options.K8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client : %w", err)
		}
		options.K8sClient = c
	}

	return &AlertingGatewayManager{
		AlertingDriverOptions: options,
	}, nil
}

func (a *AlertingGatewayManager) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	existing := a.newOpniGateway()

	err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}

	spec := extractGatewayAlertingSpec(existing)
	return &alertops.ClusterConfiguration{
		NumReplicas:             spec.Replicas,
		ClusterSettleTimeout:    spec.ClusterSettleTimeout,
		ClusterPushPullInterval: spec.ClusterPushPullInterval,
		ClusterGossipInterval:   spec.ClusterGossipInterval,
		ResourceLimits: &alertops.ResourceLimitSpec{
			Cpu:     spec.CPU,
			Memory:  spec.Memory,
			Storage: spec.Storage,
		},
	}, nil
}

func (a *AlertingGatewayManager) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	lg := a.Logger.With("action", "configure-cluster")
	lg.Debugf("%v", conf)
	existing := a.newOpniGateway()

	err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	lg.Debug("got existing gateway")
	mutator := func(gateway *corev1beta1.Gateway) {
		gateway.Spec.Alerting.Replicas = conf.NumReplicas
		gateway.Spec.Alerting.ClusterGossipInterval = conf.ClusterGossipInterval
		gateway.Spec.Alerting.ClusterPushPullInterval = conf.ClusterPushPullInterval
		gateway.Spec.Alerting.ClusterSettleTimeout = conf.ClusterSettleTimeout
		gateway.Spec.Alerting.Storage = conf.ResourceLimits.Storage
		gateway.Spec.Alerting.CPU = conf.ResourceLimits.Cpu
		gateway.Spec.Alerting.Memory = conf.ResourceLimits.Memory
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lg.Debug("Starting external update reconciler...")
		existing := a.newOpniGateway()
		err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
		lg.Debugf("updated alerting spec : %v", clone.Spec.Alerting)
		cmp, err := patch.DefaultPatchMaker.Calculate(existing, clone,
			patch.IgnoreStatusFields(),
			patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus(),
			patch.IgnorePDBSelector(),
		)
		if err == nil {
			if cmp.IsEmpty() {
				return status.Error(codes.FailedPrecondition, "no changes to apply")
			}
		}
		lg.Debug("Done cacluating external reconcile.")
		return a.K8sClient.Patch(ctx, existing, client.RawPatch(types.MergePatchType, cmp.Patch))
	})
	if retryErr != nil {
		lg.Errorf("%s", retryErr)
		return nil, retryErr
	}

	return &emptypb.Empty{}, retryErr
}

func (a *AlertingGatewayManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	existing := a.newOpniGateway()
	err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	status, err := a.alertingControllerStatus(existing)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (a *AlertingGatewayManager) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	existing := a.newOpniGateway()
	lg := a.Logger.With("action", "install-cluster")

	mutator := func(gateway *corev1beta1.Gateway) {
		gateway.Spec.Alerting.Enabled = true
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
		lg.Debugf("updated alerting spec : %v", clone.Spec.Alerting)
		cmp, err := patch.DefaultPatchMaker.Calculate(existing, clone,
			patch.IgnoreStatusFields(),
			patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus(),
			patch.IgnorePDBSelector(),
		)
		if err == nil {
			if cmp.IsEmpty() {
				return status.Error(codes.FailedPrecondition, "no changes to apply")
			}
		}
		return a.K8sClient.Patch(ctx, existing, client.RawPatch(types.MergePatchType, cmp.Patch))
	})
	if retryErr != nil {
		return nil, retryErr
	}

	return &emptypb.Empty{}, nil
}

func (a *AlertingGatewayManager) UninstallCluster(ctx context.Context, _ *alertops.UninstallRequest) (*emptypb.Empty, error) {
	existing := a.newOpniGateway()

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		existing.Spec.Alerting.Enabled = false
		return a.K8sClient.Update(ctx, existing)
	})
	if retryErr != nil {
		return nil, retryErr
	}

	return &emptypb.Empty{}, nil
}

func (a *AlertingGatewayManager) GetRuntimeOptions() shared.AlertingClusterOptions {
	return shared.AlertingClusterOptions{
		Namespace:             a.GatewayRef.Namespace,
		WorkerNodesService:    shared.OperatorAlertingClusterNodeServiceName,
		WorkerNodePort:        9093,
		ControllerNodeService: shared.OperatorAlertingControllerServiceName,
		ControllerNodePort:    9093,
		ControllerClusterPort: 9094,
		OpniPort:              3000,
	}
}

func init() {
	drivers.Drivers.Register("gateway-manager", func(_ context.Context, opts ...driverutil.Option) (drivers.ClusterDriver, error) {
		options := AlertingDriverOptions{
			GatewayRef: types.NamespacedName{
				Namespace: os.Getenv("POD_NAMESPACE"),
				Name:      os.Getenv("GATEWAY_NAME"),
			},
			ConfigKey:          shared.AlertManagerConfigKey,
			InternalRoutingKey: shared.InternalRoutingConfigKey,
			Logger:             logger.NewPluginLogger().Named("alerting").Named("alerting-manager"),
		}
		driverutil.ApplyOptions(&options, opts...)
		return NewAlertingGatewayManager(options)
	})
}

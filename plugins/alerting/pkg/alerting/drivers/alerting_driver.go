package drivers

import (
	"context"
	"fmt"
	"os"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/gogo/status"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingManager struct {
	AlertingManagerDriverOptions
	alertops.UnsafeAlertingAdminServer
	alertops.UnsafeDynamicAlertingServer
}

var _ ClusterDriver = (*AlertingManager)(nil)
var _ alertops.AlertingAdminServer = (*AlertingManager)(nil)
var _ alertops.DynamicAlertingServer = (*AlertingManager)(nil)

type AlertingManagerDriverOptions struct {
	Logger            *zap.SugaredLogger
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

func WithLogger(logger *zap.SugaredLogger) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.Logger = logger
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
	if options.k8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client : %w", err)
		}
		options.k8sClient = c
	}

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
	existing, err := a.newOpniGateway()
	if err != nil {
		return nil, err
	}

	err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}

	alertingSpec, err := extractGatewayAlertingSpec(existing)
	if err != nil {
		return nil, err
	}

	switch spec := alertingSpec.(type) {
	case *corev1beta1.AlertingSpec:
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
	case *v1beta2.AlertingSpec:
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
	default:
		return nil, fmt.Errorf("unkown alerting spec type %T", alertingSpec)
	}

}

func (a *AlertingManager) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	lg := a.Logger.With("action", "configure-cluster")
	lg.Debugf("%v", conf)
	existing, err := a.newOpniGateway()
	if err != nil {
		return nil, err
	}
	err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	lg.Debug("got existing gateway")
	mutator := func(gateway client.Object) error {
		switch gateway := gateway.(type) {
		case *corev1beta1.Gateway:
			gateway.Spec.Alerting.Replicas = conf.NumReplicas
			gateway.Spec.Alerting.ClusterGossipInterval = conf.ClusterGossipInterval
			gateway.Spec.Alerting.ClusterPushPullInterval = conf.ClusterPushPullInterval
			gateway.Spec.Alerting.ClusterSettleTimeout = conf.ClusterSettleTimeout
			gateway.Spec.Alerting.Storage = conf.ResourceLimits.Storage
			gateway.Spec.Alerting.CPU = conf.ResourceLimits.Cpu
			gateway.Spec.Alerting.Memory = conf.ResourceLimits.Memory
			return nil
		case *v1beta2.Gateway:
			gateway.Spec.Alerting.Replicas = conf.NumReplicas
			gateway.Spec.Alerting.ClusterGossipInterval = conf.ClusterGossipInterval
			gateway.Spec.Alerting.ClusterPushPullInterval = conf.ClusterPushPullInterval
			gateway.Spec.Alerting.ClusterSettleTimeout = conf.ClusterSettleTimeout
			gateway.Spec.Alerting.Storage = conf.ResourceLimits.Storage
			gateway.Spec.Alerting.CPU = conf.ResourceLimits.Cpu
			gateway.Spec.Alerting.Memory = conf.ResourceLimits.Memory
			return nil
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
	}

	retryErr := retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		lg.Debug("Starting external update reconciler...")
		existing, err := a.newOpniGateway()
		if err != nil {
			return err
		}
		err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopyObject().(client.Object)
		if err := mutator(clone); err != nil {
			return err
		}
		switch gateway := clone.(type) {
		case *corev1beta1.Gateway:
			lg.Debugf("updated alerting spec : %v", gateway.Spec.Alerting)
		case *v1beta2.Gateway:
			lg.Debugf("updated alerting spec : %v", gateway.Spec.Alerting)
		}
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
		return a.k8sClient.Update(ctx, clone)
	})
	if retryErr != nil {
		lg.Errorf("%s", retryErr)
	}
	if retryErr != nil {
		return nil, retryErr
	}

	return &emptypb.Empty{}, retryErr
}

func (a *AlertingManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	existing, err := a.newOpniGateway()
	if err != nil {
		return nil, err
	}
	err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	return a.alertingControllerStatus(existing)
}

func (a *AlertingManager) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	existing, err := a.newOpniGateway()
	if err != nil {
		return nil, err
	}
	err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	enableMutator := func(gateway client.Object) error {
		switch gateway := gateway.(type) {
		case *corev1beta1.Gateway:
			gateway.Spec.Alerting.Enabled = true
			return nil
		case *v1beta2.Gateway:
			gateway.Spec.Alerting.Enabled = true
			return nil
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
	}
	retryErr := retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		clone := existing.DeepCopyObject().(client.Object)
		if err := enableMutator(clone); err != nil {
			return err
		}
		return a.k8sClient.Update(ctx, clone)
	})
	if retryErr != nil {
		return nil, retryErr
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	existing, err := a.newOpniGateway()
	if err != nil {
		return nil, err
	}
	err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	disableMutator := func(gateway client.Object) error {
		switch gateway := gateway.(type) {
		case *corev1beta1.Gateway:
			gateway.Spec.Alerting.Enabled = false
			return nil
		case *v1beta2.Gateway:
			gateway.Spec.Alerting.Enabled = false
			return nil
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
	}
	retryErr := retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		clone := existing.DeepCopyObject().(client.Object)
		if err := disableMutator(clone); err != nil {
			return err
		}
		return a.k8sClient.Update(ctx, clone)
	})
	if retryErr != nil {
		return nil, retryErr
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) newOpniGateway() (client.Object, error) {
	switch a.gatewayApiVersion {
	case corev1beta1.GroupVersion.Identifier():

		return &corev1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      a.gatewayRef.Name,
				Namespace: a.gatewayRef.Namespace,
			},
		}, nil
	case v1beta2.GroupVersion.Identifier():
		return &v1beta2.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      a.gatewayRef.Name,
				Namespace: a.gatewayRef.Namespace,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown gateway api version %s", a.gatewayApiVersion)
	}
}

func (a *AlertingManager) newOpniControllerSet() (client.Object, error) {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName + "-internal",
			Namespace: a.gatewayRef.Namespace,
		},
	}, nil
}

func extractGatewayAlertingSpec(gw client.Object) (interface{}, error) {
	switch cl := gw.(type) {
	case *corev1beta1.Gateway:
		alerting := cl.Spec.Alerting.DeepCopy()
		return alerting, nil
	case *v1beta2.Gateway:
		alerting := cl.Spec.Alerting.DeepCopy()
		return alerting, nil
	default:
		return nil, fmt.Errorf("unknown gateway type %T", gw)
	}
}

func (a *AlertingManager) alertingControllerStatus(object client.Object) (*alertops.InstallStatus, error) {
	var isEnabled bool
	switch object := object.(type) {
	case *corev1beta1.Gateway:
		isEnabled = object.Spec.Alerting.Enabled
	case *v1beta2.Gateway:
		isEnabled = object.Spec.Alerting.Enabled
	default:
		return nil, fmt.Errorf("unknown gateway type %T", object)
	}
	ss, err := a.newOpniControllerSet()
	if err != nil {
		return nil, err
	}
	k8serr := a.k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)

	if isEnabled {
		if k8serr != nil {
			if k8serrors.IsNotFound(k8serr) {
				return &alertops.InstallStatus{
					State: alertops.InstallState_Updating,
				}, nil
			} else {
				return nil, fmt.Errorf("failed to get opni alerting controller status %w", k8serr)
			}
		}
		controller := ss.(*appsv1.StatefulSet)
		if controller.Status.Replicas != controller.Status.AvailableReplicas {
			return &alertops.InstallStatus{
				State: alertops.InstallState_Updating,
			}, nil
		}
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	} else {
		if k8serr != nil {
			if k8serrors.IsNotFound(k8serr) {
				return &alertops.InstallStatus{
					State: alertops.InstallState_NotInstalled,
				}, nil
			} else {
				return nil, fmt.Errorf("failed to get opni alerting controller status %w", k8serr)
			}
		}
		return &alertops.InstallStatus{
			State: alertops.InstallState_Uninstalling,
		}, nil
	}
}

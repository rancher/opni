package alerting_manager

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingManager struct {
	alertingOptionsMu sync.RWMutex
	AlertingManagerDriverOptions
	alertops.UnsafeAlertingAdminServer
}

func (a *AlertingManager) Name() string {
	return "alerting-manager"
}

func (a *AlertingManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

var _ drivers.ClusterDriver = (*AlertingManager)(nil)
var _ alertops.AlertingAdminServer = (*AlertingManager)(nil)

type sharedErrors struct {
	errors []error
	errMu  sync.Mutex
}

func appendError(errors *sharedErrors, err error) {
	errors.errMu.Lock()
	defer errors.errMu.Unlock()
	errors.errors = append(errors.errors, err)
}

type AlertingManagerDriverOptions struct {
	Logger             *zap.SugaredLogger
	k8sClient          client.Client
	gatewayRef         types.NamespacedName
	configKey          string
	internalRoutingKey string

	AlertingOptions *shared.AlertingClusterOptions
	Subscribers     []chan shared.AlertingClusterNotification
}

type AlertingManagerDriverOption func(*AlertingManagerDriverOptions)

func (a *AlertingManagerDriverOptions) Apply(opts ...AlertingManagerDriverOption) {
	for _, o := range opts {
		o(a)
	}
}

func WithAlertingRuntimeOptions(opts *shared.AlertingClusterOptions) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.AlertingOptions = opts
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

func WithLogger(logger *zap.SugaredLogger) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.Logger = logger
	}
}

func WithSubscribers(subscribers ...chan shared.AlertingClusterNotification) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.Subscribers = append(o.Subscribers, subscribers...)
	}
}

func NewAlertingManagerDriver(opts ...AlertingManagerDriverOption) (*AlertingManager, error) {
	options := AlertingManagerDriverOptions{
		gatewayRef: types.NamespacedName{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Name:      os.Getenv("GATEWAY_NAME"),
		},
		configKey:          shared.AlertManagerConfigKey,
		internalRoutingKey: shared.InternalRoutingConfigKey,
	}
	options.Apply(opts...)
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

func (a *AlertingManager) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	existing := a.newOpniGateway()

	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
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

func (a *AlertingManager) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	lg := a.Logger.With("action", "configure-cluster")
	lg.Debugf("%v", conf)
	existing := a.newOpniGateway()

	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
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
		err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
		lg.Debugf("updated alerting spec : %v", existing.Spec.Alerting)
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

func (a *AlertingManager) notify(status *alertops.InstallStatus) {
	for _, subscriber := range a.Subscribers {
		subscriber <- shared.AlertingClusterNotification{
			A: status.State == alertops.InstallState_Installed || status.State == alertops.InstallState_InstallUpdating,
			B: nil,
		}
	}
}

func (a *AlertingManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	existing := a.newOpniGateway()
	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	status, err := a.alertingControllerStatus(existing)
	if err != nil {
		return nil, err
	}
	a.notify(status)
	return status, nil
}

func (a *AlertingManager) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	existing := a.newOpniGateway()
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		existing.Spec.Alerting.Enabled = true
		return a.k8sClient.Update(ctx, existing)
	})
	if retryErr != nil {
		return nil, retryErr
	}

	// broadcast install hooks
	a.notify(&alertops.InstallStatus{
		State: alertops.InstallState_InstallUpdating,
	})

	for _, subscriber := range a.Subscribers {
		subscriber <- shared.AlertingClusterNotification{
			A: true,
			B: nil,
		}
	}

	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) UninstallCluster(ctx context.Context, _ *alertops.UninstallRequest) (*emptypb.Empty, error) {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := a.newOpniGateway()
		err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		existing.Spec.Alerting.Enabled = false
		return a.k8sClient.Update(ctx, existing)
	})
	if retryErr != nil {
		return nil, retryErr
	}
	// broadcast uninstall hooks
	a.notify(&alertops.InstallStatus{
		State: alertops.InstallState_Uninstalling,
	})

	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) GetRuntimeOptions() shared.AlertingClusterOptions {
	return shared.AlertingClusterOptions{
		Namespace:             a.gatewayRef.Namespace,
		WorkerNodesService:    shared.OperatorAlertingClusterNodeServiceName,
		WorkerNodePort:        9093,
		ControllerNodeService: shared.OperatorAlertingControllerServiceName,
		ControllerNodePort:    9093,
		ControllerClusterPort: 9094,
		OpniPort:              3000,
	}
}

func init() {
	drivers.RegisterClusterDriverBuilder("alerting-manager", func(_ context.Context, opts ...any) (drivers.ClusterDriver, error) {
		var options []AlertingManagerDriverOption
		for _, opt := range opts {
			switch opt := opt.(type) {
			case AlertingManagerDriverOption:
				options = append(options, opt)
			case *shared.AlertingClusterOptions:
				options = append(options, WithAlertingRuntimeOptions(opt))
			case client.Client:
				options = append(options, WithK8sClient(opt))
			case *zap.SugaredLogger:
				options = append(options, WithLogger(opt))
			case chan shared.AlertingClusterNotification:
				options = append(options, WithSubscribers(opt))
			default:
				return nil, fmt.Errorf("unknown option type %T", opt)
			}
		}
		return NewAlertingManagerDriver(options...)
	})
}

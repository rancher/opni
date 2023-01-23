package drivers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
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
	configPersistMu   sync.RWMutex
	AlertingManagerDriverOptions
	alertops.UnsafeAlertingAdminServer
}

func (a *AlertingManager) Name() string {
	return "alerting-mananger"
}

func (a *AlertingManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

func (a *AlertingManager) GetRuntimeOptions() (shared.NewAlertingOptions, error) {
	a.alertingOptionsMu.RLock()
	defer a.alertingOptionsMu.RUnlock()
	if a.AlertingOptions == nil {
		return shared.NewAlertingOptions{}, fmt.Errorf("alerting options not set")
	}
	return *a.AlertingOptions, nil
}

var _ ClusterDriver = (*AlertingManager)(nil)
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
	// ! must be mutable, as it needs to be updated on operator changes
	AlertingOptions    *shared.NewAlertingOptions
	mgmtClient         managementv1.ManagementClient
	postInstallHooks   []func()
	postUninstallHooks []func()
}

type AlertingManagerDriverOption func(*AlertingManagerDriverOptions)

func (a *AlertingManagerDriverOptions) apply(opts ...AlertingManagerDriverOption) {
	for _, o := range opts {
		o(a)
	}
}

func WithManagementClient(client managementv1.ManagementClient) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.mgmtClient = client
	}
}

func WithAlertingOptions(options *shared.NewAlertingOptions) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.AlertingOptions = options
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

func WithPostInstallHooks(postInstallHooks []func()) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.postInstallHooks = append(o.postInstallHooks, postInstallHooks...)
	}
}

func WithPostUninstallHooks(postInstallHooks []func()) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.postUninstallHooks = append(o.postUninstallHooks, postInstallHooks...)
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
	options.apply(opts...)
	if options.AlertingOptions == nil {
		return nil, fmt.Errorf("alerting options must be provided")
	}
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

func (a *AlertingManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	existing := a.newOpniGateway()
	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	return a.alertingControllerStatus(existing)
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
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(10),
		backoffv2.WithMinInterval(200*time.Millisecond),
		backoffv2.WithMaxInterval(2*time.Second),
		backoffv2.WithMultiplier(1.2),
	)
	b := retrier.Start(ctx)
	for backoffv2.Continue(b) { //FIXME: this can fail and will bust the options struct and potentially impact all the alerting functionality
		if warnErr := a.visitNewAlertingOptions(a.AlertingOptions); warnErr != nil {
			a.Logger.Warn(zap.Error(warnErr))
		} else {
			break
		}
	}
	go func() {
		for _, f := range a.postInstallHooks {
			f()
		}
	}()
	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) UninstallCluster(ctx context.Context, req *alertops.UninstallRequest) (*emptypb.Empty, error) {
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
	go func() {
		for _, f := range a.postUninstallHooks {
			f()
		}
	}()
	return &emptypb.Empty{}, nil
}

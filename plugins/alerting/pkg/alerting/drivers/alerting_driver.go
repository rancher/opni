package drivers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/config"
	"github.com/tidwall/gjson"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/gogo/status"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	appsv1 "k8s.io/api/apps/v1"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingManager struct {
	configPersistMu sync.RWMutex
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
	configKey         string
	options           *shared.NewAlertingOptions
}

type AlertingManagerDriverOption func(*AlertingManagerDriverOptions)

func (a *AlertingManagerDriverOptions) apply(opts ...AlertingManagerDriverOption) {
	for _, o := range opts {
		o(a)
	}
}

func WithAlertingOptions(options *shared.NewAlertingOptions) AlertingManagerDriverOption {
	return func(o *AlertingManagerDriverOptions) {
		o.options = options
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
		configKey:         shared.ConfigKey,
	}
	options.apply(opts...)
	if options.options == nil {
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
		existing, err := a.newOpniGateway()
		if err != nil {
			return err
		}
		err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopyObject().(client.Object)
		if err := enableMutator(clone); err != nil {
			return err
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
		existing, err := a.newOpniGateway()
		if err != nil {
			return err
		}
		err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopyObject().(client.Object)
		if err := disableMutator(clone); err != nil {
			return err
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
					State: alertops.InstallState_InstallUpdating,
				}, nil
			} else {
				return nil, fmt.Errorf("failed to get opni alerting controller status %w", k8serr)
			}
		}
		controller := ss.(*appsv1.StatefulSet)
		if controller.Status.Replicas != controller.Status.AvailableReplicas {
			return &alertops.InstallStatus{
				State: alertops.InstallState_InstallUpdating,
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

// Dynamic Server

func (a *AlertingManager) Fetch(ctx context.Context, _ *emptypb.Empty) (*alertops.AlertingConfig, error) {
	lg := a.Logger.With("action", "Fetch")
	name := a.options.ConfigMap
	namespace := a.gatewayRef.Namespace
	cfgMap := &k8scorev1.ConfigMap{}
	err := a.k8sClient.Get(
		ctx,
		client.ObjectKey{
			Name: name, Namespace: namespace},
		cfgMap)

	if err != nil || cfgMap == nil {
		msg := fmt.Sprintf("K8s runtime error, config map: %s/%s not found: %s",
			namespace,
			name,
			err)
		lg.Error(msg)
		returnErr := shared.WithInternalServerError(
			msg,
		)
		return nil, returnErr
	}
	if _, ok := cfgMap.Data[a.configKey]; !ok {
		msg := fmt.Sprintf("K8s runtime error, config map : %s key : %s not found",
			name,
			a.configKey)
		lg.Error(msg)
		return nil, shared.WithInternalServerError(
			msg,
		)
	}
	return &alertops.AlertingConfig{
		Raw: cfgMap.Data[a.configKey],
	}, nil
}

func (a *AlertingManager) Update(ctx context.Context, conf *alertops.AlertingConfig) (*emptypb.Empty, error) {
	lg := a.Logger.With("action", "Update")
	a.configPersistMu.Lock()
	defer a.configPersistMu.Unlock()
	cfgStruct := &config.ConfigMapData{}
	err := cfgStruct.Parse(conf.Raw)
	if err != nil {
		return nil, err
	}
	loopError := backend.ReconcileInvalidStateLoop(
		time.Duration(time.Second*10),
		cfgStruct,
		lg)
	if loopError != nil {
		return nil, shared.WithInternalServerError(fmt.Sprintf("failed to reconcile config : %s", loopError))
	}

	mutator := func(object client.Object) error {
		switch gateway := object.(type) {
		case *corev1beta1.Gateway:
			gateway.Spec.Alerting.RawConfigMap = conf.Raw
			return nil
		case *v1beta2.Gateway:
			gateway.Spec.Alerting.RawConfigMap = conf.Raw
			return nil
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
	}

	err = retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		existing, err := a.newOpniGateway()
		if err != nil {
			return err
		}
		err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		var clone client.Object
		switch gateway := existing.(type) {
		case *corev1beta1.Gateway:
			clone = gateway.DeepCopyObject().(client.Object)
		case *v1beta2.Gateway:
			clone = gateway.DeepCopyObject().(client.Object)
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
		if err := mutator(clone); err != nil {
			return err
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
		return a.k8sClient.Update(ctx, clone)
	})
	if err != nil {
		return nil, err
	}

	// TODO : check config map is updated before returning

	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) GetStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.DynamicStatus, error) {
	// check it has been reloaded succesfully and is running
	return nil, nil
}

func (a *AlertingManager) Reload(ctx context.Context, reloadInfo *alertops.ReloadInfo) (*emptypb.Empty, error) {
	lg := a.Logger.With("alerting-backend", "k8s", "action", "reload")
	reloadEndpoints := []string{}
	// RELOAD the controller!!!
	controllerSvcEndpoint := a.options.GetControllerEndpoint()
	reloadEndpoints = append(reloadEndpoints, controllerSvcEndpoint)
	wg := sync.WaitGroup{}
	errMtx := sync.Mutex{}
	var errors []error

	// RELOAD the workers

	name := a.options.WorkerNodesService
	namespace := a.options.Namespace
	workersEndpoints := k8scorev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(&workersEndpoints), &workersEndpoints)
	if err != nil {
		return nil, err
	}
	if len(workersEndpoints.Subsets) > 0 {
		addresses := workersEndpoints.Subsets[0].Addresses
		for _, address := range addresses {
			reloadEndpoints = append(reloadEndpoints, fmt.Sprintf("%s:%d", address.IP, a.options.WorkerNodePort))
		}
	}

	for _, endpoint := range reloadEndpoints {
		wg.Add(1)
		endpoint := endpoint
		go func() {
			defer wg.Done()
			reloadClient := &backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/reload",
				Verb:     backend.POST,
			}
			webClient := &backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/ready",
				Verb:     backend.GET,
			}
			receiverClient := (&backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/receivers",
				Verb:     backend.GET,
			}).WithHttpV2()

			// reload logic
			numReloads := 0
			numNotFound := 0
			retries := 100
			for i := 0; i < retries; i++ {
				numReloads += 1
				resp, err := http.Post(reloadClient.ConstructHTTP(), "application/json", nil)
				if err != nil {
					errMtx.Lock()
					lg.Errorf("failed to reload alertmanager %s : %s", reloadClient.Endpoint, err)
					errors = append(errors, err)
					errMtx.Unlock()
					return
				}
				if resp.StatusCode != 200 {
					errMtx.Lock()
					msg := fmt.Sprintf("failed to reload alertmanager %s successfully : %s", reloadClient.Endpoint, resp.Status)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
					return
				}
				receiverResponse, err := http.Get(receiverClient.ConstructHTTP())
				if err != nil {
					errMtx.Lock()
					msg := fmt.Sprintf("failed to fetch alertmanager receivers manually for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}
				if receiverResponse.StatusCode != 200 {
					errMtx.Lock()
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}

				body, err := io.ReadAll(receiverResponse.Body)
				if err != nil {
					errMtx.Lock()
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}
				result := gjson.Get(string(body), "#.name")
				found := false
				for _, receiver := range result.Array() {
					if receiver.String() == reloadInfo.UpdatedKey {
						found = true
						break
					}
				}
				if !found {
					numNotFound += 1
				} else {
					break
				}
				time.Sleep(time.Second * 1)
			}

			if numNotFound > 0 {
				lg.Warnf("Reloaded %s %d times, but receiver not found %d times", reloadClient.Endpoint, numReloads, numNotFound)
				if numNotFound == 100 {
					lg.Warnf("Reload likely failed for %s", reloadClient.Endpoint)
				}
			}

			for i := 0; i < 10; i++ {
				lg.Debugf("Checking alertmanager %s is ready ...", webClient.Endpoint)
				resp, err := http.Get(webClient.ConstructHTTP())
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						lg.Debugf("alertmanager %s is ready ...", webClient.Endpoint)
						return
					} else {
						lg.Warnf("Alert manager %s not ready after reload, retrying...", webClient.Endpoint)
					}
				}
				time.Sleep(time.Second)
			}
		}()
	}
	wg.Wait()
	if len(errors) > 0 {
		return nil, shared.WithInternalServerErrorf("alert backend reload failed %s", strings.Join(func() []string {
			res := []string{}
			for _, e := range errors {
				res = append(res, e.Error())
			}
			return res
		}(), ","))
	}

	return &emptypb.Empty{}, nil
}

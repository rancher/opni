package alerting_manager

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	alertingClient "github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/drivers"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingClusterManager struct {
	AlertingDriverOptions
	alertops.UnsafeAlertingAdminServer
}

var (
	_ drivers.ClusterDriver        = (*AlertingClusterManager)(nil)
	_ alertops.AlertingAdminServer = (*AlertingClusterManager)(nil)
)

var defaultConfig = &alertops.ClusterConfiguration{
	NumReplicas:             1,
	ClusterSettleTimeout:    "1m0s",
	ClusterPushPullInterval: "200ms",
	ClusterGossipInterval:   "1m0s",
	ResourceLimits: &alertops.ResourceLimitSpec{
		Cpu:    "500m",
		Memory: "200Mi",
		// This field will be deprecated with cortex alertmanager anyways, so we also hardcode it in the reconciler
		Storage: "5Gi",
	},
}

func NewAlertingClusterManager(options AlertingDriverOptions) (*AlertingClusterManager, error) {
	if options.K8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client : %w", err)
		}
		options.K8sClient = c
	}

	return &AlertingClusterManager{
		AlertingDriverOptions: options,
	}, nil
}

func (a *AlertingClusterManager) newAlertingClusterCrd() *corev1beta1.AlertingCluster {
	return &corev1beta1.AlertingCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: a.GatewayRef.Namespace,
		},
		Spec: corev1beta1.AlertingClusterSpec{
			Gateway: k8scorev1.LocalObjectReference{
				Name: a.GatewayRef.Name,
			},
		},
	}
}

func (a *AlertingClusterManager) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	lg := a.Logger.With("action", "install-cluster")
	mutator := func(cl *corev1beta1.AlertingCluster) {
		cl.Spec.Alertmanager.Enable = true
		cl.Spec.Alertmanager.ApplicationSpec.ExtraArgs = []string{
			fmt.Sprintf("--cluster.settle-timeout=%s", "1m0s"),
			fmt.Sprintf("--cluster.pushpull-interval=%s", "1m0s"),
			fmt.Sprintf("--cluster.gossip-interval=%s", "200ms"),
		}
		cl.Spec.Alertmanager.ApplicationSpec.ResourceRequirements = &k8scorev1.ResourceRequirements{
			Limits: k8scorev1.ResourceList{
				k8scorev1.ResourceCPU:    resource.MustParse("500m"),
				k8scorev1.ResourceMemory: resource.MustParse("200Mi"),
			},
		}
	}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lg.Debug("Starting external update reconciler...")
		existing := a.newAlertingClusterCrd()
		err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				mutator(existing)
				if err := a.K8sClient.Create(ctx, existing); err != nil {
					return fmt.Errorf("failed to create alerting cluster %s", err)
				}
				return nil
			}
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
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
	return &emptypb.Empty{}, nil
}

func (a *AlertingClusterManager) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	existing := a.newAlertingClusterCrd()
	if err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return defaultConfig, nil
		}
		return nil, err
	}
	clusterPushPull := "200ms"
	clusterSettle := "1m0s"
	clusterGossip := "1m0s"
	for _, arg := range existing.Spec.Alertmanager.ApplicationSpec.ExtraArgs {
		if strings.Contains("cluster.settle-timeout", arg) {
			clusterSettle = strings.Split(arg, "=")[1]
		}
		if strings.Contains("cluster.pushpull-interval", arg) {
			clusterPushPull = strings.Split(arg, "=")[1]
		}
		if strings.Contains("cluster.gossip-interval", arg) {
			clusterGossip = strings.Split(arg, "=")[1]
		}
	}
	return &alertops.ClusterConfiguration{
		NumReplicas:             lo.FromPtrOr(existing.Spec.Alertmanager.ApplicationSpec.Replicas, 1),
		ClusterSettleTimeout:    clusterSettle,
		ClusterPushPullInterval: clusterPushPull,
		ClusterGossipInterval:   clusterGossip,
		ResourceLimits: &alertops.ResourceLimitSpec{
			Cpu:    existing.Spec.Alertmanager.ApplicationSpec.ResourceRequirements.Limits.Cpu().String(),
			Memory: existing.Spec.Alertmanager.ApplicationSpec.ResourceRequirements.Limits.Memory().String(),
			// This field will be deprecated with cortex alertmanager anyways, so we also hardcode it in the reconciler
			Storage: "5Gi",
		},
	}, nil
}

func (a *AlertingClusterManager) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	lg := a.Logger.With("action", "configure-cluster")
	cpuLimit, err := resource.ParseQuantity(conf.ResourceLimits.Cpu)
	if err != nil {
		return nil, err
	}
	memLimit, err := resource.ParseQuantity(conf.ResourceLimits.Memory)
	if err != nil {
		return nil, err
	}

	mutator := func(a *corev1beta1.AlertingCluster) {
		args := []string{}
		if conf.ClusterGossipInterval != "" {
			args = append(args, fmt.Sprintf("--cluster.gossip-interval=%s", conf.ClusterGossipInterval))
		} else {
			args = append(args, fmt.Sprintf("--cluster.gossip-interval=%s", "200ms"))
		}
		if conf.ClusterPushPullInterval != "" {
			args = append(args, fmt.Sprintf("--cluster.pushpull-interval=%s", conf.ClusterPushPullInterval))
		} else {
			args = append(args, fmt.Sprintf("--cluster.pushpull-interval=%s", "1m0s"))
		}
		if conf.ClusterSettleTimeout != "" {
			args = append(args, fmt.Sprintf("--cluster.settle-timeout=%s", conf.ClusterSettleTimeout))
		} else {
			args = append(args, fmt.Sprintf("--cluster.settle-timeout=%s", "1m0s"))
		}
		a.Spec.Alertmanager.ApplicationSpec.ExtraArgs = args
		a.Spec.Alertmanager.ApplicationSpec.ResourceRequirements = &k8scorev1.ResourceRequirements{
			Limits: k8scorev1.ResourceList{
				k8scorev1.ResourceCPU:    cpuLimit,
				k8scorev1.ResourceMemory: memLimit,
			},
		}
		a.Spec.Alertmanager.ApplicationSpec.Replicas = lo.ToPtr(int32(conf.NumReplicas))
	}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lg.Debug("Starting external update reconciler...")
		existing := a.newAlertingClusterCrd()
		err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
		lg.Debugf("updated alerting spec : %v", clone.Spec)
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
	a.notify(int(conf.NumReplicas))
	return &emptypb.Empty{}, nil
}

func (a *AlertingClusterManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (status *alertops.InstallStatus, retErr error) {
	status, err := a.controllerStatus(ctx)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (a *AlertingClusterManager) controllerStatus(ctx context.Context) (*alertops.InstallStatus, error) {
	existing := a.newAlertingClusterCrd()
	if err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return &alertops.InstallStatus{
				State: alertops.InstallState_NotInstalled,
			}, nil
		}
		return nil, err
	}

	ws := newAlertmanagerSet(a.GatewayRef.Namespace)

	workErr := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)
	replicas := lo.FromPtrOr(existing.Spec.Alertmanager.ApplicationSpec.Replicas, 1)
	if existing.Spec.Alertmanager.Enable {
		expectedSets := []statusTuple{{A: workErr, B: ws}}
		for _, status := range expectedSets {
			if status.A != nil {
				if k8serrors.IsNotFound(status.A) {
					return &alertops.InstallStatus{
						State: alertops.InstallState_InstallUpdating,
					}, nil
				}
				return nil, fmt.Errorf("failed to get opni alerting status %w", status.A)
			}
			if status.B.Status.Replicas != status.B.Status.AvailableReplicas {
				return &alertops.InstallStatus{
					State: alertops.InstallState_InstallUpdating,
				}, nil
			}
		}
		// sanity check the desired number of replicas in the spec matches the total available replicas
		up := lo.Reduce(expectedSets, func(agg int32, status statusTuple, _ int) int32 {
			return agg + status.B.Status.AvailableReplicas
		}, 0)
		if up != replicas {
			return &alertops.InstallStatus{
				State: alertops.InstallState_InstallUpdating,
			}, nil
		}
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	}

	if workErr != nil {
		if k8serrors.IsNotFound(workErr) {
			return &alertops.InstallStatus{
				State: alertops.InstallState_NotInstalled,
			}, nil
		}
		return nil, fmt.Errorf("failed to get opni alerting controller status %w", workErr)
	}

	return &alertops.InstallStatus{
		State: alertops.InstallState_Uninstalling,
	}, nil
}

func (a *AlertingClusterManager) UninstallCluster(ctx context.Context, _ *alertops.UninstallRequest) (*emptypb.Empty, error) {
	cl := a.newAlertingClusterCrd()
	if err := a.K8sClient.Get(ctx, client.ObjectKeyFromObject(cl), cl); err != nil {
		if !k8serrors.IsNotFound(err) {
			return &emptypb.Empty{}, nil
		}
		return nil, err
	}
	cl.Spec.Alertmanager.Enable = false
	err := a.K8sClient.Update(ctx, cl)
	if err != nil {
		return nil, err
	}
	a.notify(0)
	return &emptypb.Empty{}, nil
}

func (a *AlertingClusterManager) Info(_ context.Context, _ *emptypb.Empty) (*alertops.ComponentInfo, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (a *AlertingClusterManager) notify(replicas int) {
	aClient, err := alertingClient.NewClient(
		alertingClient.WithAlertManagerAddress(
			fmt.Sprintf("%s:9093", shared.AlertmanagerService),
		),
		alertingClient.WithProxyAddress(
			fmt.Sprintf("%s:9093", shared.AlertmanagerService),
		),
		alertingClient.WithQuerierAddress(
			fmt.Sprintf("%s:3000", shared.AlertmanagerService),
		),
	)
	if err != nil {
		panic(err)
	}
	peers := listPeers(replicas)
	aClient.MemberlistClient().SetKnownPeers(peers)
	for _, sub := range a.Subscribers {
		sub <- aClient
	}
}

func (a *AlertingClusterManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

func (a *AlertingClusterManager) GetDefaultReceiver() *config.WebhookConfig {
	return nil
}

func listPeers(replicas int) []alertingClient.AlertingPeer {
	peers := []alertingClient.AlertingPeer{}
	for i := 0; i < replicas; i++ {
		peers = append(peers, alertingClient.AlertingPeer{
			ApiAddress:      fmt.Sprintf("http://%s-%d.%s:9093", shared.AlertmanagerService, i, shared.AlertmanagerService),
			EmbeddedAddress: fmt.Sprintf("http://%s-%d.%s:3000", shared.AlertmanagerService, i, shared.AlertmanagerService),
		})
	}
	return peers
}

func init() {
	drivers.Drivers.Register("alerting-manager", func(_ context.Context, opts ...driverutil.Option) (drivers.ClusterDriver, error) {
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
		return NewAlertingClusterManager(options)
	})
}

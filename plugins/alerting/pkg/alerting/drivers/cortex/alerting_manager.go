package cortex

import (
	"context"
	"fmt"
	"os"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/gogo/status"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/internal/cortex/config/alertmanager"
	alertopsv2 "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/pkg/alerting/shared"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// defaultConfig is a basic configuration for Cortex Alertmanager
	// that works out of the box, but is not necessarily production-ready.
	defaultConfig = &drivers.CortexAMConfiguration{
		MultitenantAlertmanagerConfig: &alertmanager.MultitenantAlertmanagerConfig{},
		StorageSpec: &storagev1.StorageSpec{
			Backend:    storagev1.Backend_filesystem,
			Filesystem: &storagev1.FilesystemStorageSpec{},
		},
		ResourceLimits: &alertopsv2.ResourceLimits{
			Memory: "200Mi",
			Cpu:    "500m",
		},
	}
)

type CortexAmOptions struct {
	GatewayRef  types.NamespacedName
	Logger      *zap.SugaredLogger
	K8sClient   client.Client
	Subscribers []chan shared.AlertingClusterNotification `option:"subscribers"`
}

var _ drivers.ClusterDriverV2 = (*CortexAmManager)(nil)

type CortexAmManager struct {
	CortexAmOptions
}

func (c *CortexAmManager) newAlertingClusterCrd() *corev1beta1.AlertingCluster {
	return &corev1beta1.AlertingCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: c.GatewayRef.Namespace,
		},
		Spec: corev1beta1.AlertingClusterSpec{
			Gateway: corev1.LocalObjectReference{
				Name: c.GatewayRef.Name,
			},
		},
	}
}

func (c *CortexAmManager) Install(ctx context.Context, conf *drivers.CortexAMConfiguration) error {
	lg := c.Logger.With("action", "install")
	mem, err := resource.ParseQuantity(conf.ResourceLimits.Memory)
	if err != nil {
		return err
	}
	cpu, err := resource.ParseQuantity(conf.ResourceLimits.Cpu)
	if err != nil {
		return err
	}
	mutator := func(cl *corev1beta1.AlertingCluster) {
		cl.Spec.Alertmanager.ApplicationSpec.StorageSpec = conf.StorageSpec
		cl.Spec.Alertmanager.ApplicationSpec.MultitenantAlertmanagerConfig = conf.MultitenantAlertmanagerConfig
		cl.Spec.Alertmanager.ApplicationSpec.ResourceRequirements = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: mem,
				corev1.ResourceCPU:    cpu,
			},
		}
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lg.Debug("Starting external update reconciler...")
		existing := c.newAlertingClusterCrd()
		err := c.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				mutator(existing)
				if err := c.K8sClient.Create(ctx, existing); err != nil {
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
		return c.K8sClient.Patch(ctx, existing, client.RawPatch(types.MergePatchType, cmp.Patch))
	})
	return retryErr
}

func (c *CortexAmManager) Configure(ctx context.Context, conf *drivers.CortexAMConfiguration) error {
	lg := c.Logger.With("action", "configure")
	mem, err := resource.ParseQuantity(conf.ResourceLimits.Memory)
	if err != nil {
		return err
	}
	cpu, err := resource.ParseQuantity(conf.ResourceLimits.Cpu)
	if err != nil {
		return err
	}
	mutator := func(cl *corev1beta1.AlertingCluster) {
		cl.Spec.Alertmanager.ApplicationSpec.StorageSpec = conf.StorageSpec
		cl.Spec.Alertmanager.ApplicationSpec.MultitenantAlertmanagerConfig = conf.MultitenantAlertmanagerConfig
		cl.Spec.Alertmanager.ApplicationSpec.ResourceRequirements = &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: mem,
				corev1.ResourceCPU:    cpu,
			},
		}
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lg.Debug("Starting external update reconciler...")
		existing := c.newAlertingClusterCrd()
		err := c.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				mutator(existing)
				if err := c.K8sClient.Create(ctx, existing); err != nil {
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
		return c.K8sClient.Patch(ctx, existing, client.RawPatch(types.MergePatchType, cmp.Patch))
	})
	return retryErr
}

func (c *CortexAmManager) GetConfiguration(ctx context.Context) (*drivers.CortexAMConfiguration, error) {
	existing := c.newAlertingClusterCrd()
	err := c.K8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return defaultConfig, nil
		}
		return nil, err
	}
	return &drivers.CortexAMConfiguration{
		StorageSpec:                   existing.Spec.Alertmanager.ApplicationSpec.StorageSpec,
		MultitenantAlertmanagerConfig: existing.Spec.Alertmanager.ApplicationSpec.MultitenantAlertmanagerConfig,
		ResourceLimits: &alertopsv2.ResourceLimits{
			Memory: existing.Spec.Alertmanager.ApplicationSpec.ResourceRequirements.Limits.Memory().String(),
			Cpu:    existing.Spec.Alertmanager.ApplicationSpec.ResourceRequirements.Limits.Cpu().String(),
		},
	}, nil
}

func (c *CortexAmManager) Status(ctx context.Context) (*alertopsv2.InstallStatus, error) {
	existing := c.newAlertingClusterCrd()
	if err := c.K8sClient.Get(context.TODO(), client.ObjectKeyFromObject(existing), existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return &alertopsv2.InstallStatus{
				State: alertopsv2.InstallState_NotInstalled,
			}, nil

		}
		return nil, err
	}
	status, err := c.statefulStatus(
		existing.Spec.Alertmanager.Enable,
		lo.FromPtrOr(existing.Spec.Alertmanager.ApplicationSpec.Replicas, 1),
	)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (c *CortexAmManager) statefulStatus(enabled bool, replicas int32) (*alertopsv2.InstallStatus, error) {
	ss := c.newCortexSS()
	ssErr := c.K8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)
	if enabled {
		if ssErr != nil {
			if k8serrors.IsNotFound(ssErr) {
				return &alertopsv2.InstallStatus{
					State: alertopsv2.InstallState_NotInstalled,
				}, nil
			}
			return nil, ssErr
		}
		if ss.Status.ReadyReplicas != replicas {
			return &alertopsv2.InstallStatus{
				State: alertopsv2.InstallState_InstallUpdating,
			}, nil
		}
		return &alertopsv2.InstallStatus{
			State: alertopsv2.InstallState_Installed,
		}, nil
	}
	if ssErr != nil {
		if k8serrors.IsNotFound(ssErr) {
			return &alertopsv2.InstallStatus{
				State: alertopsv2.InstallState_NotInstalled,
			}, nil
		}
		return nil, ssErr
	}
	return &alertopsv2.InstallStatus{
		State: alertopsv2.InstallState_Uninstalling,
	}, nil
}

func (c *CortexAmManager) newCortexSS() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: c.GatewayRef.Namespace,
		},
	}
}

func (c *CortexAmManager) Uninstall(ctx context.Context) error {
	cl := c.newAlertingClusterCrd()
	if err := c.K8sClient.Get(ctx, client.ObjectKeyFromObject(cl), cl); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	cl.Spec.Alertmanager.Enable = false
	return c.K8sClient.Update(ctx, cl)
}

func NewCortexAmManager(options CortexAmOptions) (*CortexAmManager, error) {
	if options.K8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client : %w", err)
		}
		options.K8sClient = c
	}
	return &CortexAmManager{
		CortexAmOptions: options,
	}, nil
}

func init() {
	drivers.DriversV2.Register("cortex-manager", func(_ context.Context, opts ...driverutil.Option) (drivers.ClusterDriverV2, error) {
		options := CortexAmOptions{
			GatewayRef: types.NamespacedName{
				Namespace: os.Getenv("POD_NAMESPACE"),
				Name:      os.Getenv("GATEWAY_NAME"),
			},
			Logger: logger.NewPluginLogger().Named("alerting").Named("cortex-manager"),
		}
		driverutil.ApplyOptions(&options, opts...)
		return NewCortexAmManager(options)
	})
}

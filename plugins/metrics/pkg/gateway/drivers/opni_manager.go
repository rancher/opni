package drivers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpniManager struct {
	OpniManagerClusterDriverOptions
	cortexops.UnsafeCortexOpsServer
}

type OpniManagerClusterDriverOptions struct {
	k8sClient         client.Client
	monitoringCluster types.NamespacedName
	gatewayRef        k8scorev1.LocalObjectReference
	gatewayApiVersion string
}

type OpniManagerClusterDriverOption func(*OpniManagerClusterDriverOptions)

func (o *OpniManagerClusterDriverOptions) apply(opts ...OpniManagerClusterDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) OpniManagerClusterDriverOption {
	return func(o *OpniManagerClusterDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func WithMonitoringCluster(namespacedName types.NamespacedName) OpniManagerClusterDriverOption {
	return func(o *OpniManagerClusterDriverOptions) {
		o.monitoringCluster = namespacedName
	}
}

func WithGatewayRef(gatewayRef k8scorev1.LocalObjectReference) OpniManagerClusterDriverOption {
	return func(o *OpniManagerClusterDriverOptions) {
		o.gatewayRef = gatewayRef
	}
}

func WithGatewayApiVersion(gatewayApiVersion string) OpniManagerClusterDriverOption {
	return func(o *OpniManagerClusterDriverOptions) {
		o.gatewayApiVersion = gatewayApiVersion
	}
}

func NewOpniManagerClusterDriver(opts ...OpniManagerClusterDriverOption) (*OpniManager, error) {
	options := OpniManagerClusterDriverOptions{
		monitoringCluster: types.NamespacedName{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Name:      "opni-monitoring",
		},
		gatewayRef: k8scorev1.LocalObjectReference{
			Name: os.Getenv("GATEWAY_NAME"),
		},
		gatewayApiVersion: os.Getenv("GATEWAY_API_VERSION"),
	}
	options.apply(opts...)
	if options.k8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.k8sClient = c
	}
	return &OpniManager{
		OpniManagerClusterDriverOptions: options,
	}, nil
}

var _ ClusterDriver = (*OpniManager)(nil)

func (k *OpniManager) Name() string {
	return "opni-manager"
}

func (k *OpniManager) newMonitoringCluster() (client.Object, error) {
	switch k.gatewayApiVersion {
	case corev1beta1.GroupVersion.Identifier():
		return &corev1beta1.MonitoringCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.monitoringCluster.Name,
				Namespace: k.monitoringCluster.Namespace,
			},
		}, nil
	case v1beta2.GroupVersion.Identifier():
		return &v1beta2.MonitoringCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.monitoringCluster.Name,
				Namespace: k.monitoringCluster.Namespace,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown gateway api version: %q", k.gatewayApiVersion)
	}
}

func cortexClusterStatus(object client.Object) (corev1beta1.CortexStatus, error) {
	switch object := object.(type) {
	case *corev1beta1.MonitoringCluster:
		return object.Status.Cortex, nil
	case *v1beta2.MonitoringCluster:
		return corev1beta1.CortexStatus{
			Version:        object.Status.Cortex.Version,
			WorkloadsReady: object.Status.Cortex.WorkloadsReady,
			Conditions:     object.Status.Cortex.Conditions,
			WorkloadStatus: lo.MapValues(object.Status.Cortex.WorkloadStatus, func(v v1beta2.WorkloadStatus, _ string) corev1beta1.WorkloadStatus {
				return corev1beta1.WorkloadStatus{
					Ready:   v.Ready,
					Message: v.Message,
				}
			}),
		}, nil
	default:
		return corev1beta1.CortexStatus{}, fmt.Errorf("unknown monitoring cluster type: %T", object)
	}
}

func (k *OpniManager) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*cortexops.ClusterConfiguration, error) {
	existing, err := k.newMonitoringCluster()
	if err != nil {
		return nil, err
	}
	err = k.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
	if err != nil {
		return nil, err
	}
	switch existing.GetObjectKind().GroupVersionKind().GroupVersion() {
	case corev1beta1.GroupVersion:
		mc := existing.(*corev1beta1.MonitoringCluster)
		return &cortexops.ClusterConfiguration{
			Mode:    cortexops.DeploymentMode(cortexops.DeploymentMode_value[string(mc.Spec.Cortex.DeploymentMode)]),
			Storage: mc.Spec.Cortex.Storage,
		}, nil
	case v1beta2.GroupVersion:
		mc := existing.(*v1beta2.MonitoringCluster)
		return &cortexops.ClusterConfiguration{
			Mode:    cortexops.DeploymentMode(cortexops.DeploymentMode_value[string(mc.Spec.Cortex.DeploymentMode)]),
			Storage: mc.Spec.Cortex.Storage,
		}, nil
	default:
		return nil, fmt.Errorf("unknown monitoring cluster type: %T", existing)
	}
}

func (k *OpniManager) ConfigureCluster(ctx context.Context, conf *cortexops.ClusterConfiguration) (*emptypb.Empty, error) {
	cluster, err := k.newMonitoringCluster()
	if err != nil {
		return nil, err
	}

	objectKey := client.ObjectKeyFromObject(cluster)
	err = k.k8sClient.Get(ctx, objectKey, cluster)
	exists := true
	if err != nil {
		if k8serrors.IsNotFound(err) {
			exists = false
		} else {
			return nil, fmt.Errorf("failed to get monitoring cluster: %w", err)
		}
	}

	mutator := func(cluster client.Object) {
		switch cluster := cluster.(type) {
		case *v1beta2.MonitoringCluster:
			cluster.Spec.Cortex.Enabled = true
			cluster.Spec.Cortex.Storage = conf.GetStorage()
			cluster.Spec.Grafana.Enabled = true
			cluster.Spec.Gateway = k.gatewayRef
			cluster.Spec.Cortex.DeploymentMode = v1beta2.DeploymentMode(cortexops.DeploymentMode_name[int32(conf.GetMode())])
		case *corev1beta1.MonitoringCluster:
			cluster.Spec.Cortex.Enabled = true
			cluster.Spec.Cortex.Storage = conf.GetStorage()
			cluster.Spec.Grafana.Enabled = true
			cluster.Spec.Gateway = k.gatewayRef
			cluster.Spec.Cortex.DeploymentMode = corev1beta1.DeploymentMode(cortexops.DeploymentMode_name[int32(conf.GetMode())])
		}
	}

	if exists {
		err := retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
			existing, err := k.newMonitoringCluster()
			if err != nil {
				return err
			}
			err = k.k8sClient.Get(ctx, objectKey, existing)
			if err != nil {
				return err
			}
			clone := existing.DeepCopyObject().(client.Object)
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

			return k.k8sClient.Update(ctx, clone)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update monitoring cluster: %w", err)
		}
	} else {
		mutator(cluster)
		err := k.k8sClient.Create(ctx, cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create monitoring cluster: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (k *OpniManager) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*cortexops.InstallStatus, error) {
	cluster, err := k.newMonitoringCluster()
	if err != nil {
		return nil, err
	}
	err = k.k8sClient.Get(ctx, k.monitoringCluster, cluster)
	metadata := map[string]string{}
	var state cortexops.InstallState
	var version string
	if err != nil {
		if k8serrors.IsNotFound(err) {
			state = cortexops.InstallState_NotInstalled
		} else {
			return nil, fmt.Errorf("failed to get monitoring cluster: %w", err)
		}
	} else {
		status, err := cortexClusterStatus(cluster)
		if err != nil {
			return nil, err
		}
		version = status.Version
		if cluster.GetDeletionTimestamp() != nil {
			state = cortexops.InstallState_Uninstalling
		} else {
			if status.WorkloadsReady {
				state = cortexops.InstallState_Installed
			} else {
				state = cortexops.InstallState_Updating
				metadata["Conditions"] = strings.Join(status.Conditions, "; ")
			}
		}
	}

	return &cortexops.InstallStatus{
		State:   state,
		Version: version,
		Metadata: lo.Assign(metadata, map[string]string{
			"Driver": k.Name(),
		}),
	}, nil
}

func (k *OpniManager) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	cluster, err := k.newMonitoringCluster()
	if err != nil {
		return nil, err
	}
	err = k.k8sClient.Get(ctx, k.monitoringCluster, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to uninstall monitoring cluster: %w", err)
	}

	err = k.k8sClient.Delete(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to uninstall monitoring cluster: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (k *OpniManager) ShouldDisableNode(_ *corev1.Reference) error {
	stat, err := k.GetClusterStatus(context.TODO(), &emptypb.Empty{})
	if err != nil {
		// can't determine cluster status, so don't disable the node
		return nil
	}
	switch stat.State {
	case cortexops.InstallState_NotInstalled, cortexops.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, fmt.Sprintf("Cortex cluster is not installed"))
	case cortexops.InstallState_Updating, cortexops.InstallState_Installed:
		return nil
	case cortexops.InstallState_Unknown:
		fallthrough
	default:
		// can't determine cluster status, so don't disable the node
		return nil
	}
}

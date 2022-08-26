package drivers

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesClusterDriver struct {
	KubernetesClusterDriverOptions
	cortexops.UnsafeCortexOpsServer
}

type KubernetesClusterDriverOptions struct {
	k8sClient         client.Client
	monitoringCluster types.NamespacedName
	gatewayRef        corev1.LocalObjectReference
}

type KubernetesClusterDriverOption func(*KubernetesClusterDriverOptions)

func (o *KubernetesClusterDriverOptions) apply(opts ...KubernetesClusterDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) KubernetesClusterDriverOption {
	return func(o *KubernetesClusterDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func WithMonitoringCluster(namespacedName types.NamespacedName) KubernetesClusterDriverOption {
	return func(o *KubernetesClusterDriverOptions) {
		o.monitoringCluster = namespacedName
	}
}

func WithGatewayRef(gatewayRef corev1.LocalObjectReference) KubernetesClusterDriverOption {
	return func(o *KubernetesClusterDriverOptions) {
		o.gatewayRef = gatewayRef
	}
}

func NewKubernetesClusterDriver(opts ...KubernetesClusterDriverOption) (*KubernetesClusterDriver, error) {
	options := KubernetesClusterDriverOptions{
		monitoringCluster: types.NamespacedName{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Name:      "opni-monitoring",
		},
		gatewayRef: corev1.LocalObjectReference{
			Name: os.Getenv("GATEWAY_NAME"),
		},
	}
	options.apply(opts...)
	if options.k8sClient == nil {
		c, err := util.NewK8sClient(util.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.k8sClient = c
	}
	return &KubernetesClusterDriver{
		KubernetesClusterDriverOptions: options,
	}, nil
}

var _ ClusterDriver = (*KubernetesClusterDriver)(nil)

func (k *KubernetesClusterDriver) Name() string {
	return "kubernetes"
}

func (k *KubernetesClusterDriver) ConfigureInstall(ctx context.Context, conf *cortexops.InstallConfiguration) (*emptypb.Empty, error) {
	cluster := &v1beta2.MonitoringCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.monitoringCluster.Name,
			Namespace: k.monitoringCluster.Namespace,
		},
	}
	objectKey := client.ObjectKeyFromObject(cluster)
	err := k.k8sClient.Get(ctx, objectKey, cluster)
	exists := true
	if err != nil {
		if k8serrors.IsNotFound(err) {
			exists = false
		} else {
			return nil, fmt.Errorf("failed to get monitoring cluster: %w", err)
		}
	}

	mutator := func(cluster *v1beta2.MonitoringCluster) {
		cluster.Spec.Cortex.Enabled = true
		cluster.Spec.Cortex.Storage = v1beta2.CortexStorageSpec{
			// !!!!!!
			Backend: "filesystem",
			Filesystem: &v1beta2.FilesystemStorageSpec{
				Directory: "/data",
			},
		}
		cluster.Spec.Grafana.Enabled = true
		cluster.Spec.Gateway = k.gatewayRef
		cluster.Spec.Cortex.DeploymentMode = v1beta2.DeploymentMode(cortexops.DeploymentMode_name[int32(conf.Mode)])
	}

	if exists {
		err := retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
			existing := &v1beta2.MonitoringCluster{}
			err := k.k8sClient.Get(ctx, objectKey, existing)
			if err != nil {
				return err
			}
			clone := existing.DeepCopy()
			mutator(clone)
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

func (k *KubernetesClusterDriver) GetInstallStatus(ctx context.Context, _ *emptypb.Empty) (*cortexops.InstallStatus, error) {
	cluster := &v1beta2.MonitoringCluster{}
	err := k.k8sClient.Get(ctx, k.monitoringCluster, cluster)
	var state cortexops.InstallState
	if err != nil {
		if k8serrors.IsNotFound(err) {
			state = cortexops.InstallState_NotInstalled
		} else {
			return nil, fmt.Errorf("failed to get monitoring cluster: %w", err)
		}
	} else {
		if cluster.DeletionTimestamp != nil {
			state = cortexops.InstallState_Uninstalling
		} else {
			state = cortexops.InstallState_Installed
		}
	}

	return &cortexops.InstallStatus{
		State:   state,
		Version: cluster.Annotations["opni.io/cortex-version"],
		Metadata: map[string]string{
			"Driver": k.Name(),
		},
	}, nil
}

func (k *KubernetesClusterDriver) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	cluster := &v1beta2.MonitoringCluster{}
	err := k.k8sClient.Get(ctx, k.monitoringCluster, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to uninstall monitoring cluster: %w", err)
	}

	err = k.k8sClient.Delete(ctx, cluster, client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		return nil, fmt.Errorf("failed to uninstall monitoring cluster: %w", err)
	}

	return &emptypb.Empty{}, nil
}

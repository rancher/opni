package kubernetes_manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/resources"
	k8sutilerrors "github.com/rancher/opni/pkg/util/errors/k8sutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"google.golang.org/protobuf/types/known/timestamppb"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"log/slog"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions

	ephemeralSyncTime time.Time
}

type KubernetesManagerDriverOptions struct {
	K8sClient         client.Client                  `option:"k8sClient"`
	Namespace         string                         `option:"namespace"`
	OpensearchCluster *opnimeta.OpensearchClusterRef `option:"opensearchCluster"`
	Logger            *slog.Logger                   `option:"logger"`
}

func NewKubernetesManagerDriver(options KubernetesManagerDriverOptions) (*KubernetesManagerDriver, error) {
	if options.K8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.K8sClient = c
	}
	return &KubernetesManagerDriver{
		KubernetesManagerDriverOptions: options,
		ephemeralSyncTime:              time.Now(),
	}, nil
}

func (d *KubernetesManagerDriver) GetInstallStatus(ctx context.Context) backend.InstallState {
	opensearch := &loggingv1beta1.OpniOpensearch{}
	if err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, opensearch); err != nil {
		if k8serrors.IsNotFound(err) {
			return backend.Absent
		}
		return backend.Error
	}

	cluster := &opsterv1.OpenSearchCluster{}
	if err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return backend.Pending
		}
		return backend.Error
	}
	if !cluster.Status.Initialized {
		return backend.Pending
	}

	return backend.Installed
}

func (d *KubernetesManagerDriver) StoreCluster(ctx context.Context, req *corev1.Reference, friendlyName string) error {
	_, err := d.getCluster(ctx, req.GetId())
	if err == nil {
		return loggingerrors.ErrAlreadyExists
	}

	if !errors.Is(err, loggingerrors.ErrObjectNotFound) {
		return err
	}

	labels := map[string]string{
		resources.OpniClusterID: req.GetId(),
	}

	loggingCluster := &opnicorev1beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    d.Namespace,
			Labels:       labels,
		},
		Spec: opnicorev1beta1.LoggingClusterSpec{
			OpensearchClusterRef: d.OpensearchCluster,
			FriendlyName:         friendlyName,
		},
	}

	if err := d.K8sClient.Create(ctx, loggingCluster); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to store cluster: %v", err))
		k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) getCluster(ctx context.Context, id string) (*opnicorev1beta1.LoggingCluster, error) {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list logging clusters: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}

	if len(loggingClusterList.Items) == 0 {
		return nil, loggingerrors.ErrObjectNotFound
	}

	if len(loggingClusterList.Items) > 1 {
		return nil, loggingerrors.ErrInvalidList
	}

	return &loggingClusterList.Items[0], nil
}

func (d *KubernetesManagerDriver) StoreClusterMetadata(ctx context.Context, id, name string) error {
	cluster, err := d.getCluster(ctx, id)
	if err != nil {
		return err
	}

	if cluster.Spec.FriendlyName == name {
		return nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
		if err != nil {
			return err
		}
		cluster.Spec.FriendlyName = name
		return d.K8sClient.Update(ctx, cluster)
	})

	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update cluster data: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) DeleteCluster(ctx context.Context, id string) error {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.OpensearchCluster.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list logging clusters: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	switch {
	case len(loggingClusterList.Items) > 1:
		return loggingerrors.ErrInvalidList
	case len(loggingClusterList.Items) == 1:
		loggingCluster := &loggingClusterList.Items[0]
		err := d.K8sClient.Delete(ctx, loggingCluster)
		if err != nil {
			d.Logger.Error(fmt.Sprintf("failed to delete cluster: %v", err))
			return k8sutilerrors.GRPCFromK8s(err)
		}
		return nil
	default:
		return nil
	}
}

func (d *KubernetesManagerDriver) SetClusterStatus(ctx context.Context, id string, enabled bool) error {
	syncTime := time.Now()
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list logging clusters: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	if len(loggingClusterList.Items) != 1 {
		return loggingerrors.ErrInvalidList
	}

	cluster := &loggingClusterList.Items[0]

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
		if err != nil {
			return err
		}
		cluster.Spec.LastSync = metav1.NewTime(syncTime)
		cluster.Spec.Enabled = enabled
		return d.K8sClient.Update(ctx, cluster)
	})

	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update logging cluster: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) GetClusterStatus(ctx context.Context, id string) (*capabilityv1.NodeCapabilityStatus, error) {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list logging clusters: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}

	if len(loggingClusterList.Items) == 0 {
		return &capabilityv1.NodeCapabilityStatus{
			Enabled:  false,
			LastSync: timestamppb.New(d.ephemeralSyncTime),
		}, nil
	}

	if len(loggingClusterList.Items) != 1 {
		return nil, loggingerrors.ErrInvalidList
	}

	return &capabilityv1.NodeCapabilityStatus{
		Enabled:  loggingClusterList.Items[0].Spec.Enabled,
		LastSync: timestamppb.New(loggingClusterList.Items[0].Spec.LastSync.Time),
	}, nil
}

func (d *KubernetesManagerDriver) SetSyncTime() {
	d.ephemeralSyncTime = time.Now()
}

func (d *KubernetesManagerDriver) StoreClusterReadUser(ctx context.Context, username, password, id string) error {
	user := &loggingv1beta1.MulticlusterUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      username,
			Namespace: d.Namespace,
		},
		Spec: loggingv1beta1.MulticlusterUserSpec{
			Password:             password,
			OpensearchClusterRef: d.OpensearchCluster,
		},
	}

	cluster, err := d.getCluster(ctx, id)
	if err != nil {
		return err
	}

	controllerutil.SetOwnerReference(cluster, user, d.K8sClient.Scheme())
	err = client.IgnoreAlreadyExists(d.K8sClient.Create(ctx, user))
	if err != nil {
		return err
	}

	binding := &loggingv1beta1.LoggingClusterBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("support-%s", username),
			Namespace: d.Namespace,
		},
		Spec: loggingv1beta1.LoggingClusterBindingSpec{
			MulticlusterUser: &loggingv1beta1.MulticlusterUserRef{
				Name:      username,
				Namespace: d.Namespace,
			},
			LoggingCluster: &loggingv1beta1.LoggingClusterRef{
				ID: id,
			},
			OpensearchClusterRef: d.OpensearchCluster,
		},
	}
	controllerutil.SetOwnerReference(cluster, binding, d.K8sClient.Scheme())

	err = client.IgnoreAlreadyExists(d.K8sClient.Create(ctx, binding))

	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to store logging cluster binding: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func init() {
	backend.Drivers.Register("kubernetes-manager", func(_ context.Context, opts ...driverutil.Option) (backend.ClusterDriver, error) {
		options := KubernetesManagerDriverOptions{
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      "opni",
				Namespace: os.Getenv("POD_NAMESPACE"),
			},
			Namespace: os.Getenv("POD_NAMESPACE"),
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}
		return NewKubernetesManagerDriver(options)
	})
}

package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions

	ephemeralSyncTime time.Time
}

type KubernetesManagerDriverOptions struct {
	k8sClient         client.Client
	namespace         string
	opensearchCluster *opnimeta.OpensearchClusterRef
	logger            *zap.SugaredLogger
}

type KubernetesManagerDriverOption func(*KubernetesManagerDriverOptions)

func (o *KubernetesManagerDriverOptions) apply(opts ...KubernetesManagerDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) KubernetesManagerDriverOption {
	return func(o *KubernetesManagerDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func WithNamespace(namespace string) KubernetesManagerDriverOption {
	return func(o *KubernetesManagerDriverOptions) {
		o.namespace = namespace
	}
}

func WithOpensearchCluster(opensearchCluster *opnimeta.OpensearchClusterRef) KubernetesManagerDriverOption {
	return func(o *KubernetesManagerDriverOptions) {
		o.opensearchCluster = opensearchCluster
	}
}
func WithLogger(logger *zap.SugaredLogger) KubernetesManagerDriverOption {
	return func(o *KubernetesManagerDriverOptions) {
		o.logger = logger
	}
}

func NewKubernetesManagerDriver(opts ...KubernetesManagerDriverOption) (*KubernetesManagerDriver, error) {
	options := KubernetesManagerDriverOptions{
		opensearchCluster: &opnimeta.OpensearchClusterRef{
			Name:      "opni",
			Namespace: os.Getenv("POD_NAMESPACE"),
		},
		namespace: os.Getenv("POD_NAMESPACE"),
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
	return &KubernetesManagerDriver{
		KubernetesManagerDriverOptions: options,
		ephemeralSyncTime:              time.Now(),
	}, nil
}

func (d *KubernetesManagerDriver) Name() string {
	return "kubernetes-manager"
}

func (d *KubernetesManagerDriver) GetInstallStatus(ctx context.Context) InstallState {
	opensearch := &loggingv1beta1.OpniOpensearch{}
	if err := d.k8sClient.Get(ctx, types.NamespacedName{
		Name:      d.opensearchCluster.Name,
		Namespace: d.opensearchCluster.Namespace,
	}, opensearch); err != nil {
		if k8serrors.IsNotFound(err) {
			return Absent
		}
		return Error
	}

	cluster := &opsterv1.OpenSearchCluster{}
	if err := d.k8sClient.Get(ctx, types.NamespacedName{
		Name:      d.opensearchCluster.Name,
		Namespace: d.opensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return Pending
		}
		return Error
	}
	if !cluster.Status.Initialized {
		return Pending
	}

	return Installed
}

func (d *KubernetesManagerDriver) StoreCluster(ctx context.Context, req *corev1.Reference) error {
	labels := map[string]string{
		resources.OpniClusterID: req.GetId(),
	}
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.k8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.namespace),
		client.MatchingLabels{resources.OpniClusterID: req.GetId()},
	); err != nil {
		return errors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 0 {
		return errors.ErrCreateFailedAlreadyExists(req.GetId())
	}

	loggingCluster := &opnicorev1beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    d.namespace,
			Labels:       labels,
		},
		Spec: opnicorev1beta1.LoggingClusterSpec{
			OpensearchClusterRef: d.opensearchCluster,
		},
	}

	if err := d.k8sClient.Create(ctx, loggingCluster); err != nil {
		errors.ErrStoreClusterFailed(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) DeleteCluster(ctx context.Context, id string) error {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.k8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.opensearchCluster.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		errors.ErrListingClustersFaled(err)
	}

	switch {
	case len(loggingClusterList.Items) > 1:
		return errors.ErrDeleteClusterInvalidList(id)
	case len(loggingClusterList.Items) == 1:
		loggingCluster := &loggingClusterList.Items[0]
		return d.k8sClient.Delete(ctx, loggingCluster)
	default:
		return nil
	}
}

func (d *KubernetesManagerDriver) SetClusterStatus(ctx context.Context, id string, enabled bool) error {
	syncTime := time.Now()
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.k8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		return errors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) != 1 {
		return errors.ErrInvalidList
	}

	cluster := &loggingClusterList.Items[0]

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
		if err != nil {
			return err
		}
		cluster.Spec.LastSync = metav1.NewTime(syncTime)
		cluster.Spec.Enabled = enabled
		return d.k8sClient.Update(ctx, cluster)
	})
}

func (d *KubernetesManagerDriver) GetClusterStatus(ctx context.Context, id string) (*capabilityv1.NodeCapabilityStatus, error) {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := d.k8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(d.namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		return nil, errors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) == 0 {
		return &capabilityv1.NodeCapabilityStatus{
			Enabled:  false,
			LastSync: timestamppb.New(d.ephemeralSyncTime),
		}, nil
	}

	if len(loggingClusterList.Items) != 1 {
		return nil, errors.ErrInvalidList
	}

	return &capabilityv1.NodeCapabilityStatus{
		Enabled:  loggingClusterList.Items[0].Spec.Enabled,
		LastSync: timestamppb.New(loggingClusterList.Items[0].Spec.LastSync.Time),
	}, nil
}

func (d *KubernetesManagerDriver) SetSyncTime() {
	d.ephemeralSyncTime = time.Now()
}

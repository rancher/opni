package drivers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions
}

type KubernetesManagerDriverOptions struct {
	k8sClient         client.Client
	namespace         string
	opensearchCluster *opnimeta.OpensearchClusterRef
	logger            zap.SugaredLogger
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
func WithLogger(logger zap.SugaredLogger) KubernetesManagerDriverOption {
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
	}, nil
}

func (d *KubernetesManagerDriver) Name() string {
	return "kubernetes-manager"
}

func (d *KubernetesManagerDriver) CreateCredentials(ctx context.Context, req *opnicorev1.Reference) error {
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

	// Generate credentials
	userSuffix := string(util.GenerateRandomString(6))

	password := string(util.GenerateRandomString(20))

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("index-%s", strings.ToLower(userSuffix)),
			Namespace: d.namespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			"password": password,
		},
	}

	if err := d.k8sClient.Create(ctx, userSecret); err != nil {
		errors.ErrStoreUserCredentialsFailed(err)
	}

	loggingCluster := &opnicorev1beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    d.namespace,
			Labels:       labels,
		},
		Spec: opnicorev1beta1.LoggingClusterSpec{
			IndexUserSecret: &corev1.LocalObjectReference{
				Name: userSecret.Name,
			},
			OpensearchClusterRef: d.opensearchCluster,
		},
	}

	if err := d.k8sClient.Create(ctx, loggingCluster); err != nil {
		errors.ErrStoreClusterFailed(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) GetCredentials(ctx context.Context, id string) (username string, password string) {
	labels := map[string]string{
		resources.OpniClusterID: id,
	}
	secrets := &corev1.SecretList{}
	if err := d.k8sClient.List(ctx, secrets, client.InNamespace(d.namespace), client.MatchingLabels(labels)); err != nil {
		d.logger.Errorf("unable to list secrets: %v", err)
		return
	}

	if len(secrets.Items) != 1 {
		d.logger.Error("no credential secrets found")
		return
	}

	username = secrets.Items[0].Name
	password = string(secrets.Items[0].Data["password"])
	return
}

func (d *KubernetesManagerDriver) GetExternalURL(ctx context.Context) string {
	opnimgmt := &loggingv1beta1.OpniOpensearch{}
	if err := d.k8sClient.Get(ctx, types.NamespacedName{
		Name:      d.opensearchCluster.Name,
		Namespace: d.namespace,
	}, opnimgmt); err != nil {
		d.logger.Errorf("unable to fetch opniopensearch object: %v", err)
		return ""
	}

	return opnimgmt.Spec.ExternalURL
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
	return Installed
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

	if len(loggingClusterList.Items) != 1 {
		return nil, errors.ErrInvalidList
	}

	return &capabilityv1.NodeCapabilityStatus{
		Enabled:  loggingClusterList.Items[0].Spec.Enabled,
		LastSync: timestamppb.New(loggingClusterList.Items[0].Spec.LastSync.Time),
	}, nil
}

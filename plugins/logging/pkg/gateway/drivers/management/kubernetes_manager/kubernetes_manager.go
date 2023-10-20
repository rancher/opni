package kubernetes_manager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	k8sutilerrors "github.com/rancher/opni/pkg/util/errors/k8sutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"log/slog"
	opsterv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelOpsterCluster  = "opster.io/opensearch-cluster"
	LabelOpsterNodePool = "opster.io/opensearch-nodepool"
	LabelOpniNodeGroup  = "opni.io/node-group"
	TopologyKeyK8sHost  = "kubernetes.io/hostname"

	opensearchVersion   = "2.8.0"
	defaultRepo         = "docker.io/rancher"
	s3CredentialsSecret = "opni-opensearch-s3"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions
}

type KubernetesManagerDriverOptions struct {
	OpensearchCluster *opnimeta.OpensearchClusterRef `option:"opensearchCluster"`
	K8sClient         client.Client                  `option:"k8sClient"`
	Logger            *slog.Logger
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
	}, nil
}

func (d *KubernetesManagerDriver) AdminPassword(ctx context.Context) (password []byte, retErr error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}

	retErr = d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, k8sOpensearchCluster)
	if retErr != nil {
		d.Logger.Error("failed to get opensearch cluster")
		retErr = k8sutilerrors.GRPCFromK8s(retErr)
		return
	}

	password = util.GenerateRandomString(12)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-user-password",
			Namespace: d.OpensearchCluster.Namespace,
		},
		Data: map[string][]byte{
			"password": password,
		},
	}

	ctrl.SetControllerReference(k8sOpensearchCluster, secret, d.K8sClient.Scheme())
	retErr = d.K8sClient.Create(ctx, secret)
	if retErr != nil {
		if !k8serrors.IsAlreadyExists(retErr) {
			d.Logger.Error(fmt.Sprintf("failed to create secret: %v", retErr))
			retErr = k8sutilerrors.GRPCFromK8s(retErr)
			return
		}
		retErr = d.K8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if retErr != nil {
			d.Logger.Error(fmt.Sprintf("failed to get existing secret: %v", retErr))
			retErr = k8sutilerrors.GRPCFromK8s(retErr)
			return
		}
		password = secret.Data["password"]
		return
	}
	return
}

func (d *KubernetesManagerDriver) NewOpensearchClientForCluster(ctx context.Context) *opensearch.Client {
	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(ctx)

	cluster := &opsterv1.OpenSearchCluster{}

FETCH:
	for {
		select {
		case <-b.Done():
			return nil
		case <-b.Next():
			err := d.K8sClient.Get(ctx, types.NamespacedName{
				Name:      d.OpensearchCluster.Name,
				Namespace: d.OpensearchCluster.Namespace,
			}, cluster)
			if err != nil {
				continue
			}
			break FETCH
		}
	}

	username, _, err := helpers.UsernameAndPassword(ctx, d.K8sClient, cluster)
	if err != nil {
		panic(err)
	}

	certMgr := certs.NewCertMgrOpensearchCertManager(
		ctx,
		certs.WithNamespace(cluster.Namespace),
		certs.WithCluster(cluster.Name),
	)

	client, err := opensearch.NewClient(
		opensearch.ClientConfig{
			URLs: []string{
				fmt.Sprintf("https://%s:9200", cluster.Spec.General.ServiceName),
			},
			Username:   username,
			CertReader: certMgr,
		},
	)
	if err != nil {
		panic(err)
	}

	return client
}

func (d *KubernetesManagerDriver) DeleteCluster(ctx context.Context) error {
	loggingClusters := &opnicorev1beta1.LoggingClusterList{}
	err := d.K8sClient.List(ctx, loggingClusters, client.InNamespace(d.OpensearchCluster.Namespace))
	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list logging clusters: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	if len(loggingClusters.Items) > 0 {
		return loggingerrors.ErrLoggingCapabilityExists
	}

	cluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.OpensearchCluster.Name,
			Namespace: d.OpensearchCluster.Namespace,
		},
	}

	err = d.K8sClient.Delete(ctx, cluster)
	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to delete cluster: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) GetCluster(ctx context.Context) (*loggingadmin.OpensearchClusterV2, error) {
	cluster := &loggingv1beta1.OpniOpensearch{}
	if err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return &loggingadmin.OpensearchClusterV2{}, nil
		}
		d.Logger.Error(fmt.Sprintf("failed to fetch cluster: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}

	data, err := generateDataDetails(cluster.Spec.NodePools)
	if err != nil {
		return nil, err
	}

	controlplane, err := generateControlplaneDetails(cluster.Spec.NodePools)
	if err != nil {
		return nil, err
	}

	return &loggingadmin.OpensearchClusterV2{
		ExternalURL:       cluster.Spec.ExternalURL,
		DataNodes:         data,
		IngestNodes:       generateIngestDetails(cluster.Spec.NodePools),
		ControlplaneNodes: controlplane,
		Dashboards:        convertDashboardsToProtobuf(cluster.Spec.Dashboards),
		DataRetention:     &cluster.Spec.IndexRetention,
		S3:                d.s3ToProtobuf(ctx, cluster.Spec.S3Settings),
	}, nil
}

func (d *KubernetesManagerDriver) CreateOrUpdateCluster(
	ctx context.Context,
	cluster *loggingadmin.OpensearchClusterV2,
	opniVersion string,
	natName string,
) error {
	err := d.storeS3Credentials(ctx, cluster.GetS3().GetCredentials())
	if err != nil {
		return err
	}

	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}
	exists := true
	err = d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			d.Logger.Error(fmt.Sprintf("failed to fetch cluster: %v", err))
			return k8sutilerrors.GRPCFromK8s(err)
		}
		exists = false
	}

	nodePools, err := d.generateNodePools(cluster)
	if err != nil {
		return err
	}

	if !exists {
		k8sOpensearchCluster = &loggingv1beta1.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      d.OpensearchCluster.Name,
				Namespace: d.OpensearchCluster.Namespace,
			},
			Spec: loggingv1beta1.OpniOpensearchSpec{
				OpensearchSettings: loggingv1beta1.OpensearchSettings{
					Dashboards: convertProtobufToDashboards(cluster.Dashboards, nil, opniVersion),
					NodePools:  nodePools,
					Security: &opsterv1.Security{
						Tls: &opsterv1.TlsConfig{
							Transport: &opsterv1.TlsConfigTransport{
								Generate: true,
								PerNode:  true,
							},
							Http: &opsterv1.TlsConfigHttp{
								Generate: true,
							},
						},
					},
					S3Settings: s3ToKubernetes(cluster.GetS3()),
				},
				ExternalURL: cluster.ExternalURL,
				ClusterConfigSpec: &loggingv1beta1.ClusterConfigSpec{
					IndexRetention: lo.FromPtrOr(cluster.DataRetention, "7d"),
				},
				OpensearchVersion: opensearchVersion,
				Version:           opniVersion,
				ImageRepo:         "docker.io/rancher",
				NatsRef: &corev1.LocalObjectReference{
					Name: natName,
				},
			},
		}

		err = d.K8sClient.Create(ctx, k8sOpensearchCluster)
		if err != nil {
			d.Logger.Error(fmt.Sprintf("failed to create cluster: %v", err))
			return k8sutilerrors.GRPCFromK8s(err)
		}
		return nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.OpensearchSettings.NodePools = nodePools
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards = convertProtobufToDashboards(
			cluster.Dashboards,
			k8sOpensearchCluster,
			opniVersion,
		)
		k8sOpensearchCluster.Spec.OpensearchSettings.S3Settings = s3ToKubernetes(cluster.GetS3())
		k8sOpensearchCluster.Spec.ExternalURL = cluster.ExternalURL
		if cluster.DataRetention != nil {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = &loggingv1beta1.ClusterConfigSpec{
				IndexRetention: *cluster.DataRetention,
			}
		} else {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = nil
		}

		return d.K8sClient.Update(ctx, k8sOpensearchCluster)
	})

	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update cluster: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) UpgradeAvailable(ctx context.Context, opniVersion string) (bool, error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}

	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			d.Logger.Error("opensearch cluster does not exist")
			return false, loggingerrors.WrappedGetPrereqFailed(err)
		}
		d.Logger.Error(fmt.Sprintf("failed to fetch opensearch cluster: %v", err))
		return false, k8sutilerrors.GRPCFromK8s(err)
	}

	if k8sOpensearchCluster.Status.Version == nil || k8sOpensearchCluster.Status.OpensearchVersion == nil {
		return false, nil
	}

	if *k8sOpensearchCluster.Status.Version != opniVersion {
		return true, nil
	}
	if *k8sOpensearchCluster.Status.OpensearchVersion != opensearchVersion {
		return true, nil
	}

	return false, nil
}

func (d *KubernetesManagerDriver) DoUpgrade(ctx context.Context, opniVersion string) error {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.OpensearchCluster.Name,
			Namespace: d.OpensearchCluster.Namespace,
		},
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}

		k8sOpensearchCluster.Spec.Version = opniVersion
		k8sOpensearchCluster.Spec.OpensearchVersion = opensearchVersion

		image := fmt.Sprintf(
			"%s/opensearch-dashboards:v%s-%s",
			defaultRepo,
			opniVersion,
			opensearchVersion,
		)

		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Image = &image
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Version = opensearchVersion

		return d.K8sClient.Update(ctx, k8sOpensearchCluster)
	})
	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update opensearch cluster: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) GetStorageClasses(ctx context.Context) ([]string, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := d.K8sClient.List(ctx, storageClasses); err != nil {
		d.Logger.Error(fmt.Sprintf("failed to list storageclasses: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}

	storageClassNames := make([]string, 0, len(storageClasses.Items))
	for _, storageClass := range storageClasses.Items {
		storageClassNames = append(storageClassNames, storageClass.Name)
	}

	return storageClassNames, nil
}

func (d *KubernetesManagerDriver) CreateOrUpdateSnapshotSchedule(
	ctx context.Context,
	snapshot *loggingadmin.SnapshotSchedule,
	defaultIndices []string,
) error {
	repo := &loggingv1beta1.OpensearchRepository{}
	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, repo)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			d.Logger.Error("opensearch repository does not exist")
			return loggingerrors.WrappedGetPrereqFailed(err)
		}
		d.Logger.Error(fmt.Sprintf("failed to list opensearch repositories: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	return d.createOrUpdateRecurringSnapshot(ctx, snapshot, defaultIndices, repo)
}

func (d *KubernetesManagerDriver) GetSnapshotSchedule(
	ctx context.Context,
	ref *loggingadmin.SnapshotReference,
	defaultIndices []string,
) (*loggingadmin.SnapshotSchedule, error) {
	snapshot := &loggingv1beta1.RecurringSnapshot{}
	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      ref.GetName(),
		Namespace: d.OpensearchCluster.Namespace,
	}, snapshot)
	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to fetch snapshot: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}

	return &loggingadmin.SnapshotSchedule{
		Ref:          ref,
		CronSchedule: snapshot.Spec.Creation.CronSchedule,
		Retention: func() *loggingadmin.SnapshotRetention {
			if snapshot.Spec.Retention == nil {
				return nil
			}
			return &loggingadmin.SnapshotRetention{
				TimeRetention: func() *string {
					if snapshot.Spec.Retention.MaxAge == "" {
						return nil
					}
					return &snapshot.Spec.Retention.MaxAge
				}(),
				MaxSnapshots: snapshot.Spec.Retention.MaxCount,
			}
		}(),
		AdditionalIndices: loggingutil.RemoveFromSlice(snapshot.Spec.Snapshot.Indices, defaultIndices),
	}, nil
}

func (d *KubernetesManagerDriver) DeleteSnapshotSchedule(ctx context.Context, ref *loggingadmin.SnapshotReference) error {
	err := d.K8sClient.Delete(ctx, &loggingv1beta1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.GetName(),
			Namespace: d.OpensearchCluster.Namespace,
		},
	})
	if client.IgnoreNotFound(err) != nil {
		d.Logger.Error(fmt.Sprintf("failed to delete snapshot: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	err = d.K8sClient.Delete(ctx, &loggingv1beta1.RecurringSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.GetName(),
			Namespace: d.OpensearchCluster.Namespace,
		},
	})
	if client.IgnoreNotFound(err) != nil {
		d.Logger.Error(fmt.Sprintf("failed to delete snapshot: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	return nil
}

func (d *KubernetesManagerDriver) ListAllSnapshotSchedules(ctx context.Context) (*loggingadmin.SnapshotStatusList, error) {
	recurring, err := d.listRecurringSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	return &loggingadmin.SnapshotStatusList{
		Statuses: recurring,
	}, nil
}

func init() {
	management.Drivers.Register("kubernetes-manager", func(_ context.Context, opts ...driverutil.Option) (management.ClusterDriver, error) {
		options := KubernetesManagerDriverOptions{
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      "opni",
				Namespace: os.Getenv("POD_NAMESPACE"),
			},
		}
		driverutil.ApplyOptions(&options, opts...)
		return NewKubernetesManagerDriver(options)
	})
}

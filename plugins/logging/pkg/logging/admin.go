package logging

import (
	"context"
	"fmt"

	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelOpsterCluster  = "opster.io/opensearch-cluster"
	LabelOpsterNodePool = "opster.io/opensearch-nodepool"
	TopologyKeyK8sHost  = "kubernetes.io/hostname"

	opensearchVersion     = "1.3.3"
	opensearchClusterName = "opni"
	defaultRepo           = "docker.io/rancher"
)

func (p *Plugin) GetOpensearchCluster(
	ctx context.Context,
	empty *emptypb.Empty,
) (*loggingadmin.OpensearchCluster, error) {
	cluster := &opniv1beta2.OpniOpensearch{}
	if err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      opensearchClusterName,
		Namespace: p.storageNamespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			p.logger.Info("opensearch cluster does not exist")
			return nil, nil
		}
		return nil, err
	}

	var nodePools []*loggingadmin.OpensearchNodeDetails
	for _, pool := range cluster.Spec.OpensearchSettings.NodePools {
		convertedPool, err := convertNodePoolToProtobuf(pool)
		if err != nil {
			return nil, err
		}
		nodePools = append(nodePools, convertedPool)
	}

	dashboards := convertDashboardsToProtobuf(cluster.Spec.OpensearchSettings.Dashboards)

	return &loggingadmin.OpensearchCluster{
		ExternalURL: cluster.Spec.ExternalURL,
		DataRetention: func() *string {
			if cluster.Spec.ClusterConfigSpec == nil {
				return nil
			}
			if cluster.Spec.ClusterConfigSpec.IndexRetention == "" {
				return nil
			}
			return &cluster.Spec.ClusterConfigSpec.IndexRetention
		}(),
		NodePools:  nodePools,
		Dashboards: dashboards,
	}, nil
}

func (p *Plugin) DeleteOpensearchCluster(
	ctx context.Context,
	empty *emptypb.Empty,
) (*emptypb.Empty, error) {

	cluster := &opniv1beta2.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opensearchClusterName,
			Namespace: p.storageNamespace,
		},
	}
	return nil, p.k8sClient.Delete(ctx, cluster)
}

func (p *Plugin) CreateOrUpdateOpensearchCluster(
	ctx context.Context,
	cluster *loggingadmin.OpensearchCluster,
) (*emptypb.Empty, error) {
	k8sOpensearchCluster := &opniv1beta2.OpniOpensearch{}

	exists := true
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      opensearchClusterName,
		Namespace: p.storageNamespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		exists = false
	}

	var nodePools []opsterv1.NodePool
	for _, pool := range cluster.NodePools {
		convertedPool, err := convertProtobufToNodePool(pool, opensearchClusterName)
		if err != nil {
			return nil, err
		}
		nodePools = append(nodePools, convertedPool)
	}

	if !exists {
		k8sOpensearchCluster = &opniv1beta2.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opensearchClusterName,
				Namespace: p.storageNamespace,
			},
			Spec: opniv1beta2.OpniOpensearchSpec{
				OpensearchSettings: opniv1beta2.OpensearchSettings{
					Dashboards: p.convertProtobufToDashboards(cluster.Dashboards, k8sOpensearchCluster),
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
				},
				ExternalURL: cluster.ExternalURL,
				ClusterConfigSpec: &opniv1beta2.ClusterConfigSpec{
					IndexRetention: lo.FromPtrOr(cluster.DataRetention, "7d"),
				},
				OpensearchVersion: opensearchVersion,
				Version: func() string {
					if p.version != "" {
						return p.version
					}
					return util.Version
				}(),
			},
		}
		return nil, p.k8sClient.Create(ctx, k8sOpensearchCluster)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := p.k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.OpensearchSettings.NodePools = nodePools
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards = p.convertProtobufToDashboards(cluster.Dashboards, k8sOpensearchCluster)
		k8sOpensearchCluster.Spec.ExternalURL = cluster.ExternalURL
		if cluster.DataRetention != nil {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = &opniv1beta2.ClusterConfigSpec{
				IndexRetention: *cluster.DataRetention,
			}
		} else {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = nil
		}

		return p.k8sClient.Update(ctx, k8sOpensearchCluster)
	})

	return nil, err
}

func (p *Plugin) UpgradeAvailable(context.Context, *emptypb.Empty) (*loggingadmin.UpgradeAvailableResponse, error) {
	k8sOpensearchCluster := &opniv1beta2.OpniOpensearch{}
	var version string
	version = util.Version
	if p.version != "" {
		version = p.version
	}

	err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      opensearchClusterName,
		Namespace: p.storageNamespace,
	}, k8sOpensearchCluster)
	if err != nil {
		return nil, err
	}

	if k8sOpensearchCluster.Status.Version == nil || k8sOpensearchCluster.Status.OpensearchVersion == nil {
		return &loggingadmin.UpgradeAvailableResponse{
			UpgradePending: false,
		}, nil
	}

	if *k8sOpensearchCluster.Status.Version != version {
		return &loggingadmin.UpgradeAvailableResponse{
			UpgradePending: true,
		}, nil
	}
	if *k8sOpensearchCluster.Status.OpensearchVersion != opensearchVersion {
		return &loggingadmin.UpgradeAvailableResponse{
			UpgradePending: true,
		}, nil
	}

	return &loggingadmin.UpgradeAvailableResponse{
		UpgradePending: false,
	}, nil
}

func (p *Plugin) DoUpgrade(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	k8sOpensearchCluster := &opniv1beta2.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opensearchClusterName,
			Namespace: p.storageNamespace,
		},
	}

	var version string
	version = util.Version
	if p.version != "" {
		version = p.version
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := p.k8sClient.Get(p.ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}

		k8sOpensearchCluster.Spec.Version = version
		k8sOpensearchCluster.Spec.OpensearchVersion = opensearchVersion

		image := fmt.Sprintf(
			"%s/opensearch-dashboards:%s-%s",
			defaultRepo,
			opensearchVersion,
			version,
		)

		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Image = &image
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Version = opensearchVersion

		return p.k8sClient.Update(p.ctx, k8sOpensearchCluster)
	})

	return nil, err
}

func convertNodePoolToProtobuf(pool opsterv1.NodePool) (*loggingadmin.OpensearchNodeDetails, error) {
	diskSize := resource.MustParse(pool.DiskSize)

	var tolerations []*corev1.Toleration
	for _, toleration := range pool.Tolerations {
		tolerations = append(tolerations, &toleration)
	}

	persistence := loggingadmin.DataPersistence{}
	if pool.Persistence == nil {
		persistence.Enabled = lo.ToPtr(true)
	} else {
		if pool.Persistence.EmptyDir != nil {
			persistence.Enabled = lo.ToPtr(false)
		} else {
			if pool.Persistence.PVC != nil {
				persistence.Enabled = lo.ToPtr(true)
				persistence.StorageClass = func() *string {
					if pool.Persistence.PVC.StorageClassName == "" {
						return nil
					}
					return &pool.Persistence.PVC.StorageClassName
				}()
			} else {
				return &loggingadmin.OpensearchNodeDetails{}, ErrStoredClusterPersistence()
			}
		}
	}

	return &loggingadmin.OpensearchNodeDetails{
		Name:        pool.Component,
		Replicas:    &pool.Replicas,
		DiskSize:    &diskSize,
		MemoryLimit: pool.Resources.Limits.Memory(),
		CPUResources: func() *loggingadmin.CPUResource {
			var request *resource.Quantity
			var limit *resource.Quantity

			if !pool.Resources.Requests.Cpu().IsZero() {
				request = pool.Resources.Requests.Cpu()
			}
			if !pool.Resources.Limits.Cpu().IsZero() {
				limit = pool.Resources.Limits.Cpu()
			}

			if request == nil && limit == nil {
				return nil
			}
			return &loggingadmin.CPUResource{
				Request: pool.Resources.Requests.Cpu(),
				Limit:   pool.Resources.Limits.Cpu(),
			}
		}(),
		NodeSelector: pool.NodeSelector,
		Tolerations:  tolerations,
		Persistence:  &persistence,
		Roles:        pool.Roles,
	}, nil
}

func (p *Plugin) convertProtobufToDashboards(
	dashboard *loggingadmin.DashboardsDetails,
	cluster *opniv1beta2.OpniOpensearch,
) opsterv1.DashboardsConfig {
	var version, osVersion string
	if cluster == nil {
		version = util.Version
		if p.version != "" {
			version = p.version
		}
		osVersion = opensearchVersion
	} else {
		if cluster.Status.Version != nil {
			version = *cluster.Status.Version
		} else {
			version = util.Version
			if p.version != "" {
				version = p.version
			}
		}
		if cluster.Status.OpensearchVersion != nil {
			osVersion = *cluster.Status.OpensearchVersion
		} else {
			osVersion = opensearchVersion
		}
	}

	image := fmt.Sprintf(
		"%s/opensearch-dashboards:%s-%s",
		defaultRepo,
		osVersion,
		version,
	)

	return opsterv1.DashboardsConfig{
		ImageSpec: &opsterv1.ImageSpec{
			Image: &image,
		},
		Replicas: lo.FromPtrOr(dashboard.Replicas, 1),
		Enable:   lo.FromPtrOr(dashboard.Enabled, true),
		Resources: func() corev1.ResourceRequirements {
			if dashboard.Resources == nil {
				return corev1.ResourceRequirements{}
			}
			return *dashboard.Resources
		}(),
		Version: osVersion,
		Tls: &opsterv1.DashboardsTlsConfig{
			Enable:   true,
			Generate: true,
		},
		AdditionalConfig: map[string]string{
			"opensearchDashboards.branding.applicationTitle":        "Opni Logging",
			"opensearchDashboards.branding.faviconUrl":              "https://raw.githubusercontent.com/rancher/opni/main/branding/favicon.png",
			"opensearchDashboards.branding.loadingLogo.darkModeUrl": "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading-dark.svg",
			"opensearchDashboards.branding.loadingLogo.defaultUrl":  "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading.svg",
			"opensearchDashboards.branding.logo.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-logo-dark.svg",
			"opensearchDashboards.branding.mark.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-mark.svg",
		},
	}
}

func convertProtobufToNodePool(pool *loggingadmin.OpensearchNodeDetails, clusterName string) (opsterv1.NodePool, error) {
	if pool.MemoryLimit == nil {
		return opsterv1.NodePool{}, ErrRequestMissingMemory()
	}

	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: *pool.MemoryLimit,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: *pool.MemoryLimit,
		},
	}

	if pool.CPUResources != nil {
		if pool.CPUResources.Request != nil {
			resources.Requests[corev1.ResourceCPU] = *pool.CPUResources.Request
		}
		if pool.CPUResources.Limit != nil {
			resources.Requests[corev1.ResourceCPU] = *pool.CPUResources.Limit
		}
	}

	jvmVal := pool.MemoryLimit.Value() / 2

	return opsterv1.NodePool{
		Component: pool.Name,
		Replicas:  lo.FromPtrOr(pool.Replicas, 1),
		DiskSize:  pool.DiskSize.String(),
		Resources: resources,
		Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", jvmVal, jvmVal),
		Roles:     pool.Roles,
		Tolerations: func() []corev1.Toleration {
			var tolerations []corev1.Toleration
			for _, toleration := range pool.Tolerations {
				if toleration != nil {
					tolerations = append(tolerations, *toleration)
				}
			}
			return tolerations
		}(),
		NodeSelector: pool.NodeSelector,
		Affinity: func() *corev1.Affinity {
			if lo.FromPtrOr(pool.EnableAntiAffinity, true) {
				return &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										LabelOpsterCluster:  clusterName,
										LabelOpsterNodePool: pool.Name,
									},
								},
								TopologyKey: TopologyKeyK8sHost,
							},
						},
					},
				}
			}
			return nil
		}(),
		Persistence: func() *opsterv1.PersistenceConfig {
			if pool.Persistence == nil {
				return nil
			}
			if lo.FromPtrOr(pool.Persistence.Enabled, true) {
				return &opsterv1.PersistenceConfig{
					PersistenceSource: opsterv1.PersistenceSource{
						PVC: &opsterv1.PVCSource{
							StorageClassName: lo.FromPtrOr(pool.Persistence.StorageClass, ""),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
						},
					},
				}
			}
			return &opsterv1.PersistenceConfig{
				PersistenceSource: opsterv1.PersistenceSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}
		}(),
	}, nil
}

func convertDashboardsToProtobuf(dashboard opsterv1.DashboardsConfig) *loggingadmin.DashboardsDetails {
	return &loggingadmin.DashboardsDetails{
		Enabled:  &dashboard.Enable,
		Replicas: &dashboard.Replicas,
		Resources: func() *corev1.ResourceRequirements {
			if dashboard.Resources.Limits == nil && dashboard.Resources.Requests == nil {
				return nil
			}
			return &dashboard.Resources
		}(),
	}
}

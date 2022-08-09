package logging

import (
	"context"
	"fmt"

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

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
)

const (
	LabelOpsterCluster  = "opster.io/opensearch-cluster"
	LabelOpsterNodePool = "opster.io/opensearch-nodepool"
	TopologyKeyK8sHost  = "kubernetes.io/hostname"

	opensearchVersion     = "1.3.3"
	opensearchClusterName = "opni"
)

var (
	dashboardsImage = fmt.Sprintf("rancher/opensearch-dashboards:%s-%s", opensearchVersion, util.Version)
	opensearchImage = fmt.Sprintf("rancher/opensearch:%s-%s", opensearchVersion, util.Version)
)

func (p *Plugin) GetOpensearchCluster(
	ctx context.Context,
	empty *emptypb.Empty,
) (*loggingadmin.OpensearchCluster, error) {
	cluster := &opsterv1.OpenSearchCluster{}
	if err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      opensearchClusterName,
		Namespace: p.storageNamespace,
	}, cluster); err != nil {
		return nil, err
	}

	var nodePools []*loggingadmin.OpensearchNodeDetails
	for _, pool := range cluster.Spec.NodePools {
		convertedPool, err := convertNodePoolToProtobuf(pool)
		if err != nil {
			return nil, err
		}
		nodePools = append(nodePools, convertedPool)
	}

	dashboards := convertDashboardsToProtobuf(cluster.Spec.Dashboards)

	return &loggingadmin.OpensearchCluster{
		NodePools:  nodePools,
		Dashboards: dashboards,
	}, nil
}

func (p *Plugin) DeleteOpensearchCluster(
	ctx context.Context,
	empty *emptypb.Empty,
) (*emptypb.Empty, error) {
	cluster := &opsterv1.OpenSearchCluster{
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
	k8sOpensearchCluster := &opsterv1.OpenSearchCluster{}

	exists := true
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      opensearchClusterName,
		Namespace: p.storageNamespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			exists = false
		}
		return nil, err
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
		k8sOpensearchCluster = &opsterv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opensearchClusterName,
				Namespace: p.storageNamespace,
			},
			Spec: opsterv1.ClusterSpec{
				Dashboards: convertProtobufToDashboards(cluster.Dashboards),
				NodePools:  nodePools,
				General: opsterv1.GeneralConfig{
					ImageSpec: &opsterv1.ImageSpec{
						Image: &opensearchImage,
					},
					Version:          opensearchVersion,
					ServiceName:      fmt.Sprintf("%s-opensearch-svc", opensearchClusterName),
					HttpPort:         9200,
					SetVMMaxMapCount: true,
				},
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
		}
		return nil, p.k8sClient.Create(ctx, k8sOpensearchCluster)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := p.k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.NodePools = nodePools
		k8sOpensearchCluster.Spec.Dashboards = convertProtobufToDashboards(cluster.Dashboards)

		return p.k8sClient.Update(ctx, k8sOpensearchCluster)
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
	}
	if pool.Persistence.EmptyDir != nil {
		persistence.Enabled = lo.ToPtr(false)
	} else {
		if pool.Persistence.PVC != nil {
			persistence.Enabled = lo.ToPtr(true)
			persistence.StorageClass = &pool.Persistence.PVC.StorageClassName
		} else {
			return &loggingadmin.OpensearchNodeDetails{}, ErrStoredClusterPersistence()
		}
	}

	return &loggingadmin.OpensearchNodeDetails{
		Name:        pool.Component,
		Replicas:    &pool.Replicas,
		DiskSize:    &diskSize,
		MemoryLimit: pool.Resources.Limits.Memory(),
		CPUResources: &loggingadmin.CPUResource{
			Request: pool.Resources.Requests.Cpu(),
			Limit:   pool.Resources.Limits.Cpu(),
		},
		NodeSelector: pool.NodeSelector,
		Tolerations:  tolerations,
		Persistence:  &persistence,
	}, nil
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
			if !lo.FromPtrOr(pool.Persistence.Enabled, true) {
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
		Enabled:   &dashboard.Enable,
		Replicas:  &dashboard.Replicas,
		Resources: &dashboard.Resources,
	}
}

func convertProtobufToDashboards(dashboard *loggingadmin.DashboardsDetails) opsterv1.DashboardsConfig {
	return opsterv1.DashboardsConfig{
		ImageSpec: &opsterv1.ImageSpec{
			Image: &dashboardsImage,
		},
		Replicas: lo.FromPtrOr(dashboard.Replicas, 1),
		Enable:   lo.FromPtrOr(dashboard.Enabled, true),
		Resources: func() corev1.ResourceRequirements {
			if dashboard.Resources == nil {
				return corev1.ResourceRequirements{}
			}
			return *dashboard.Resources
		}(),
		Version: opensearchVersion,
		Tls: &opsterv1.DashboardsTlsConfig{
			Enable:   true,
			Generate: true,
		},
		AdditionalConfig: map[string]string{
			"opensearchDashboards.branding.logo.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-logo-dark.svg",
			"opensearchDashboards.branding.mark.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-mark.svg",
			"opensearchDashboards.branding.loadingLogo.defaultUrl":  "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading.svg",
			"opensearchDashboards.branding.loadingLogo.darkModeUrl": "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading-dark.svg",
			"opensearchDashboards.branding.faviconUrl":              "https://raw.githubusercontent.com/rancher/opni/main/branding/favicon.png",
			"opensearchDashboards.branding.applicationTitle":        "Opni Logging",
		},
	}
}

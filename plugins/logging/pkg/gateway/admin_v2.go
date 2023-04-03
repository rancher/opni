package gateway

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/util"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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

	opensearchVersion = "2.4.0"
	defaultRepo       = "docker.io/rancher"
)

type ClusterStatus int

const (
	ClusterStatusPending ClusterStatus = iota + 1
	ClusterStatusGreen
	ClusterStatusYellow
	ClusterStatusRed
	ClusterStatusError
)

func ClusterStatusDescription(s ClusterStatus) string {
	switch s {
	case ClusterStatusPending:
		return "Opensearch cluster is initializing"
	case ClusterStatusGreen:
		return "Opensearch cluster is green"
	case ClusterStatusYellow:
		return "Opensearch cluster is yellow"
	case ClusterStatusRed:
		return "Opensearch cluster is red"
	case ClusterStatusError:
		return "Error fetching status from Opensearch cluster"
	default:
		return "unknown status"
	}
}

type LoggingManagerV2 struct {
	loggingadmin.UnsafeLoggingAdminV2Server
	k8sClient         client.Client
	logger            *zap.SugaredLogger
	opensearchCluster *opnimeta.OpensearchClusterRef
	opensearchManager *opensearchdata.Manager
	otelForwarder     *otel.OTELForwarder
	storageNamespace  string
	natsRef           *corev1.LocalObjectReference
	versionOverride   string
}

func (m *LoggingManagerV2) GetOpensearchCluster(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.OpensearchClusterV2, error) {
	cluster := &loggingv1beta1.OpniOpensearch{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.opensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			m.logger.Info("opensearch cluster does not exist")
			return &loggingadmin.OpensearchClusterV2{}, nil
		}
		return nil, err
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
	}, nil
}

func (m *LoggingManagerV2) DeleteOpensearchCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// Check that it is safe to delete the cluster
	m.opensearchManager.UnsetClient()

	// Remove the state tracking the initial admin
	err := m.opensearchManager.DeleteInitialAdminState()
	if err != nil {
		return nil, err
	}

	loggingClusters := &opnicorev1beta1.LoggingClusterList{}
	err = m.k8sClient.List(ctx, loggingClusters, client.InNamespace(m.storageNamespace))
	if err != nil {
		return nil, err
	}

	if len(loggingClusters.Items) > 0 {
		m.logger.Error("can not delete opensearch until logging capability is uninstalled from all clusters")
		return nil, errors.ErrLoggingCapabilityExists
	}

	cluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.opensearchCluster.Name,
			Namespace: m.storageNamespace,
		},
	}

	err = m.k8sClient.Delete(ctx, cluster)
	if err != nil {
		return nil, err
	}

	err = m.k8sClient.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-user-password",
			Namespace: m.storageNamespace,
		},
	})
	if err != nil {
		m.logger.Errorf("failed to cleanup user secret: %s", err)
	}

	return &emptypb.Empty{}, nil
}

func (m *LoggingManagerV2) CreateOrUpdateOpensearchCluster(ctx context.Context, cluster *loggingadmin.OpensearchClusterV2) (*emptypb.Empty, error) {
	// Validate retention string
	if !m.validDurationString(lo.FromPtrOr(cluster.DataRetention, "7d")) {
		return &emptypb.Empty{}, errors.ErrInvalidRetention()
	}
	// Input should always have a data nodes field
	if cluster.GetDataNodes() == nil {
		return &emptypb.Empty{}, errors.ErrMissingDataNode()
	}

	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}

	go m.opensearchManager.SetClient(m.setOpensearchClient)
	m.otelForwarder.BackgroundInitClient()

	exists := true
	err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.storageNamespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		exists = false
	}

	nodePools, err := m.generateNodePools(cluster)
	if err != nil {
		return nil, err
	}

	if !exists {
		k8sOpensearchCluster = &loggingv1beta1.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      m.opensearchCluster.Name,
				Namespace: m.storageNamespace,
			},
			Spec: loggingv1beta1.OpniOpensearchSpec{
				OpensearchSettings: loggingv1beta1.OpensearchSettings{
					Dashboards: m.convertProtobufToDashboards(cluster.Dashboards, nil),
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
				ClusterConfigSpec: &loggingv1beta1.ClusterConfigSpec{
					IndexRetention: lo.FromPtrOr(cluster.DataRetention, "7d"),
				},
				OpensearchVersion: opensearchVersion,
				Version: func() string {
					if m.versionOverride != "" {
						return m.versionOverride
					}
					return strings.TrimPrefix(versions.Version, "v")
				}(),
				ImageRepo: "docker.io/rancher",
				NatsRef:   m.natsRef,
			},
		}

		err = m.k8sClient.Create(ctx, k8sOpensearchCluster)
		if err != nil {
			return nil, err
		}

		err := m.createInitialAdmin()
		if err != nil {
			return nil, err
		}

		return &emptypb.Empty{}, nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.OpensearchSettings.NodePools = nodePools
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards = m.convertProtobufToDashboards(cluster.Dashboards, k8sOpensearchCluster)
		k8sOpensearchCluster.Spec.ExternalURL = cluster.ExternalURL
		if cluster.DataRetention != nil {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = &loggingv1beta1.ClusterConfigSpec{
				IndexRetention: *cluster.DataRetention,
			}
		} else {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = nil
		}

		return m.k8sClient.Update(ctx, k8sOpensearchCluster)
	})

	return &emptypb.Empty{}, err
}

func (m *LoggingManagerV2) UpgradeAvailable(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.UpgradeAvailableResponse, error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}
	version := strings.TrimPrefix(versions.Version, "v")

	err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.storageNamespace,
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

func (m *LoggingManagerV2) DoUpgrade(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.opensearchCluster.Name,
			Namespace: m.storageNamespace,
		},
	}

	version := strings.TrimPrefix(versions.Version, "v")

	if m.versionOverride != "" {
		version = m.versionOverride
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
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

		return m.k8sClient.Update(ctx, k8sOpensearchCluster)
	})

	return &emptypb.Empty{}, err
}

func (m *LoggingManagerV2) GetStorageClasses(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.StorageClassResponse, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := m.k8sClient.List(ctx, storageClasses); err != nil {
		return nil, err
	}

	storageClassNames := make([]string, 0, len(storageClasses.Items))
	for _, storageClass := range storageClasses.Items {
		storageClassNames = append(storageClassNames, storageClass.Name)
	}

	return &loggingadmin.StorageClassResponse{
		StorageClasses: storageClassNames,
	}, nil
}

func (m *LoggingManagerV2) GetOpensearchStatus(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.StatusResponse, error) {
	if err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.opensearchCluster.Namespace,
	}, &loggingv1beta1.OpniOpensearch{}); err != nil {
		if k8serrors.IsNotFound(err) {
			m.logger.Info("opensearch cluster does not exist")
			return nil, status.Error(codes.NotFound, "unable to list cluster status")
		}
		return nil, err
	}

	status := ClusterStatus(-1)

	cluster := &opsterv1.OpenSearchCluster{}
	if err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.opensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			status = ClusterStatusPending
			return &loggingadmin.StatusResponse{
				Status:  int32(status),
				Details: ClusterStatusDescription(status),
			}, nil
		}
		return nil, err
	}

	if !cluster.Status.Initialized {
		status = ClusterStatusPending
		return &loggingadmin.StatusResponse{
			Status:  int32(status),
			Details: ClusterStatusDescription(status),
		}, nil
	}

	statusResp := m.opensearchManager.GetClusterStatus()
	switch statusResp {
	case opensearchdata.ClusterStatusGreen:
		status = ClusterStatusGreen
	case opensearchdata.ClusterStatusYellow:
		status = ClusterStatusYellow
	case opensearchdata.ClusterStatusRed:
		status = ClusterStatusRed
	case opensearchdata.ClusterStatusError:
		status = ClusterStatusError
	}

	return &loggingadmin.StatusResponse{
		Status:  int32(status),
		Details: ClusterStatusDescription(status),
	}, nil
}

func generateDataDetails(pools []opsterv1.NodePool) (*loggingadmin.DataDetails, error) {
	var (
		referencePool opsterv1.NodePool
		replicas      int32
		tolerations   []*corev1.Toleration
	)

	for _, pool := range pools {
		if slices.Contains(pool.Roles, "data") {
			replicas += pool.Replicas
			referencePool = pool
		}
	}

	for _, toleration := range referencePool.Tolerations {
		tolerations = append(tolerations, &toleration)
	}

	persistence, err := generatePersistence(referencePool)
	if err != nil {
		return &loggingadmin.DataDetails{}, err
	}

	return &loggingadmin.DataDetails{
		Replicas:           &replicas,
		DiskSize:           referencePool.DiskSize,
		MemoryLimit:        referencePool.Resources.Limits.Memory().String(),
		CpuResources:       generateCPU(referencePool),
		EnableAntiAffinity: antiAffinityEnabled(referencePool),
		NodeSelector:       referencePool.NodeSelector,
		Tolerations:        tolerations,
		Persistence:        persistence,
	}, nil

}

func generateIngestDetails(pools []opsterv1.NodePool) *loggingadmin.IngestDetails {
	var (
		referencePool opsterv1.NodePool
		replicas      int32
		tolerations   []*corev1.Toleration
	)

	for _, pool := range pools {
		if slices.Contains(pool.Roles, "ingest") {
			if slices.Contains(pool.Roles, "data") {
				return nil
			}
			replicas += pool.Replicas
			referencePool = pool
		}
	}

	for _, toleration := range referencePool.Tolerations {
		tolerations = append(tolerations, &toleration)
	}

	return &loggingadmin.IngestDetails{
		Replicas:           &replicas,
		MemoryLimit:        referencePool.Resources.Limits.Memory().String(),
		CpuResources:       generateCPU(referencePool),
		EnableAntiAffinity: antiAffinityEnabled(referencePool),
		NodeSelector:       referencePool.NodeSelector,
		Tolerations:        tolerations,
	}
}

func generateControlplaneDetails(pools []opsterv1.NodePool) (*loggingadmin.ControlplaneDetails, error) {
	var (
		referencePool opsterv1.NodePool
		tolerations   []*corev1.Toleration
	)

	for _, pool := range pools {
		if slices.Contains(pool.Roles, "master") {
			if slices.Contains(pool.Roles, "data") {
				return nil, nil
			}
			referencePool = pool
		}
	}

	for _, toleration := range referencePool.Tolerations {
		tolerations = append(tolerations, &toleration)
	}

	persistence, err := generatePersistence(referencePool)
	if err != nil {
		return &loggingadmin.ControlplaneDetails{}, err
	}

	return &loggingadmin.ControlplaneDetails{
		Replicas:     &referencePool.Replicas,
		NodeSelector: referencePool.NodeSelector,
		Tolerations:  tolerations,
		Persistence:  persistence,
	}, nil
}

func generatePersistence(pool opsterv1.NodePool) (*loggingadmin.DataPersistence, error) {
	persistence := &loggingadmin.DataPersistence{}
	if pool.Persistence == nil {
		persistence.Enabled = lo.ToPtr(true)
		return persistence, nil
	}

	if pool.Persistence.EmptyDir != nil {
		persistence.Enabled = lo.ToPtr(false)
		return persistence, nil
	}

	if pool.Persistence.PVC != nil {
		persistence.Enabled = lo.ToPtr(true)
		persistence.StorageClass = func() *string {
			if pool.Persistence.PVC.StorageClassName == "" {
				return nil
			}
			return &pool.Persistence.PVC.StorageClassName
		}()
		return persistence, nil
	}

	return persistence, errors.ErrStoredClusterPersistence()
}

func generateCPU(pool opsterv1.NodePool) *loggingadmin.CPUResource {
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
		Request: pool.Resources.Requests.Cpu().String(),
		Limit:   pool.Resources.Limits.Cpu().String(),
	}
}

func antiAffinityEnabled(pool opsterv1.NodePool) *bool {
	if pool.Affinity == nil {
		return lo.ToPtr(false)
	}
	enabled := pool.Affinity.PodAntiAffinity != nil
	return &enabled
}

func (m *LoggingManagerV2) generateNodePools(cluster *loggingadmin.OpensearchClusterV2) (pools []opsterv1.NodePool, retErr error) {
	resources, jvm, retErr := generateK8sResources(
		cluster.GetDataNodes().GetMemoryLimit(),
		cluster.GetDataNodes().GetCpuResources(),
	)
	if retErr != nil {
		return
	}

	if lo.FromPtrOr(cluster.DataNodes.Replicas, 1) < 1 {
		retErr = errors.ErrReplicasZero("data")
		return
	}

	initialPool := opsterv1.NodePool{
		Component: "data",
		Replicas:  lo.FromPtrOr(cluster.DataNodes.Replicas, 1),
		Labels: map[string]string{
			LabelOpniNodeGroup: "data",
		},
		DiskSize:     cluster.GetDataNodes().GetDiskSize(),
		NodeSelector: cluster.GetDataNodes().GetNodeSelector(),
		Resources:    resources,
		Jvm:          jvm,
		Tolerations:  convertTolerations(cluster.GetDataNodes().GetTolerations()),
		Affinity: func() *corev1.Affinity {
			if lo.FromPtrOr(cluster.GetDataNodes().EnableAntiAffinity, false) {
				return m.generateAntiAffinity("data")
			}
			return nil
		}(),
		Persistence: convertPersistence(cluster.GetDataNodes().GetPersistence()),
		Env: []corev1.EnvVar{
			{
				Name:  "DISABLE_INSTALL_DEMO_CONFIG",
				Value: "true",
			},
		},
	}

	var extraControlPlanePool, splitPool bool
	roles := []string{
		"data",
	}
	if cluster.GetIngestNodes() == nil {
		roles = append(roles, "ingest")
	}
	if cluster.GetControlplaneNodes() == nil {
		roles = append(roles, "master")
		if lo.FromPtrOr(cluster.DataNodes.Replicas, 1)%2 == 0 || lo.FromPtrOr(cluster.DataNodes.Replicas, 1) < 3 {
			extraControlPlanePool = true
		}
		if lo.FromPtrOr(cluster.DataNodes.Replicas, 1) > 5 {
			splitPool = true
			extraControlPlanePool = false
			initialPool.Replicas = 5
		}
	}

	initialPool.Roles = roles
	pools = append(pools, initialPool)

	if splitPool {
		secondPool := initialPool.DeepCopy()
		secondPool.Roles = util.RemoveFirstOccurence(secondPool.Roles, "master")
		secondPool.Replicas = *cluster.DataNodes.Replicas - 5
		secondPool.Component = "datax"
		pools = append(pools, *secondPool)
	}

	if extraControlPlanePool {
		_, jvm, err := generateK8sResources("512Mi", &loggingadmin.CPUResource{
			Request: "100m",
		})
		if err != nil {
			retErr = err
			return
		}
		resources, _, err := generateK8sResources("640Mi", &loggingadmin.CPUResource{
			Request: "100m",
		})
		if err != nil {
			retErr = err
			return
		}
		cpPool := opsterv1.NodePool{
			Component: "quorum",
			Replicas: func() int32 {
				if lo.FromPtrOr(cluster.DataNodes.Replicas, 1) < 3 {
					return 3 - lo.FromPtrOr(cluster.DataNodes.Replicas, 1)
				}
				return 1
			}(),
			Roles: []string{
				"master",
			},
			Labels: map[string]string{
				LabelOpniNodeGroup: "data",
			},
			DiskSize:     "5Gi",
			NodeSelector: cluster.DataNodes.NodeSelector,
			Resources:    resources,
			Jvm:          jvm,
			Tolerations:  convertTolerations(cluster.DataNodes.Tolerations),
			Affinity: func() *corev1.Affinity {
				if lo.FromPtrOr(cluster.DataNodes.EnableAntiAffinity, false) {
					return m.generateAntiAffinity("data")
				}
				return nil
			}(),
			Env: []corev1.EnvVar{
				{
					Name:  "DISABLE_INSTALL_DEMO_CONFIG",
					Value: "true",
				},
				{
					Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
					Value: "true",
				},
			},
		}
		pools = append(pools, cpPool)
	}

	if cluster.GetIngestNodes() != nil {
		ingest, err := m.convertIngestDetails(cluster.IngestNodes)
		if err != nil {
			retErr = err
			return
		}
		pools = append(pools, ingest)
	}

	if cluster.GetControlplaneNodes() != nil {
		cp, err := m.convertControlplaneDetails(cluster.ControlplaneNodes)
		if err != nil {
			retErr = err
			return
		}
		pools = append(pools, cp)
	}

	return
}

func generateK8sResources(memoryLimit string, cpu *loggingadmin.CPUResource) (corev1.ResourceRequirements, string, error) {
	if memoryLimit == "" {
		return corev1.ResourceRequirements{}, "", errors.ErrRequestMissingMemory()
	}

	memory, err := resource.ParseQuantity(memoryLimit)
	if err != nil {
		return corev1.ResourceRequirements{}, "", err
	}

	jvmVal := memory.Value() / 2

	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: memory,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: memory,
		},
	}

	if cpu != nil {
		var request, limit resource.Quantity
		if cpu.Request != "" {
			request, err = resource.ParseQuantity(cpu.Request)
			if err != nil {
				return corev1.ResourceRequirements{}, "", err
			}
			resources.Requests[corev1.ResourceCPU] = request
		}
		if cpu.Limit != "" {
			limit, err = resource.ParseQuantity(cpu.Limit)
			if err != nil {
				return corev1.ResourceRequirements{}, "", err
			}
			resources.Limits[corev1.ResourceCPU] = limit
		}
		if cpu.Request != "" && cpu.Limit != "" {
			if request.Cmp(limit) > 0 {
				return corev1.ResourceRequirements{}, "", errors.ErrRequestGtLimits()
			}
		}
	}
	jvm := fmt.Sprintf("-Xmx%d -Xms%d", jvmVal, jvmVal)
	return resources, jvm, nil
}

func convertTolerations(pointers []*corev1.Toleration) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, toleration := range pointers {
		if toleration != nil {
			tolerations = append(tolerations, *toleration)
		}
	}
	return tolerations
}

func (m *LoggingManagerV2) generateAntiAffinity(opniRole string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								LabelOpsterCluster: m.opensearchCluster.Name,
								LabelOpniNodeGroup: opniRole,
							},
						},
						TopologyKey: TopologyKeyK8sHost,
					},
				},
			},
		},
	}
}

func convertPersistence(persistence *loggingadmin.DataPersistence) *opsterv1.PersistenceConfig {
	if persistence == nil {
		return nil
	}
	if lo.FromPtrOr(persistence.Enabled, true) {
		return &opsterv1.PersistenceConfig{
			PersistenceSource: opsterv1.PersistenceSource{
				PVC: &opsterv1.PVCSource{
					StorageClassName: lo.FromPtrOr(persistence.StorageClass, ""),
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
}

func (m *LoggingManagerV2) convertIngestDetails(details *loggingadmin.IngestDetails) (opsterv1.NodePool, error) {
	resources, jvm, err := generateK8sResources(details.MemoryLimit, details.CpuResources)
	if err != nil {
		return opsterv1.NodePool{}, err
	}

	if lo.FromPtrOr(details.Replicas, 1) < 1 {
		return opsterv1.NodePool{}, errors.ErrReplicasZero("ingest")
	}

	return opsterv1.NodePool{
		Component: "ingest",
		Roles: []string{
			"ingest",
		},
		Replicas: lo.FromPtrOr(details.Replicas, 1),
		Labels: map[string]string{
			LabelOpniNodeGroup: "ingest",
		},
		DiskSize:     "5Gi",
		NodeSelector: details.NodeSelector,
		Resources:    resources,
		Jvm:          jvm,
		Tolerations:  convertTolerations(details.Tolerations),
		Affinity: func() *corev1.Affinity {
			if lo.FromPtrOr(details.EnableAntiAffinity, false) {
				return m.generateAntiAffinity("ingest")
			}
			return nil
		}(),
		Persistence: &opsterv1.PersistenceConfig{
			PersistenceSource: opsterv1.PersistenceSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "DISABLE_INSTALL_DEMO_CONFIG",
				Value: "true",
			},
		},
	}, nil
}

func (m *LoggingManagerV2) convertControlplaneDetails(details *loggingadmin.ControlplaneDetails) (opsterv1.NodePool, error) {
	_, jvm, err := generateK8sResources("512Mi", &loggingadmin.CPUResource{
		Request: "100m",
	})
	if err != nil {
		return opsterv1.NodePool{}, err
	}

	if lo.FromPtrOr(details.Replicas, 1) < 1 {
		return opsterv1.NodePool{}, errors.ErrReplicasZero("controlplane")
	}

	// Give the controlplane nodes slightly more memory that double the jvm
	// testing has shown the GC can spike at lower memory levels
	resources, _, err := generateK8sResources("640Mi", &loggingadmin.CPUResource{
		Request: "100m",
	})
	if err != nil {
		return opsterv1.NodePool{}, err
	}

	return opsterv1.NodePool{
		Component: "controlplane",
		Roles: []string{
			"master",
		},
		Replicas: lo.FromPtrOr(details.Replicas, 3),
		Labels: map[string]string{
			LabelOpniNodeGroup: "controlplane",
		},
		DiskSize:     "5Gi",
		NodeSelector: details.NodeSelector,
		Resources:    resources,
		Jvm:          jvm,
		Tolerations:  convertTolerations(details.Tolerations),
		Affinity:     m.generateAntiAffinity("controlplane"),
		Persistence:  convertPersistence(details.Persistence),
		Env: []corev1.EnvVar{
			{
				Name:  "DISABLE_INSTALL_DEMO_CONFIG",
				Value: "true",
			},
			{
				Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
				Value: "true",
			},
		},
	}, nil
}

func (m *LoggingManagerV2) validDurationString(duration string) bool {
	match, err := regexp.MatchString(`^\d+[dMmyh]`, duration)
	if err != nil {
		m.logger.Errorf("could not run regexp: %v", err)
		return false
	}
	return match
}

func (m *LoggingManagerV2) opensearchClusterReady() bool {
	ctx := context.TODO()
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
			m.logger.Warn("plugin context cancelled before Opensearch object created")
			return true
		case <-b.Next():
			err := m.k8sClient.Get(ctx, types.NamespacedName{
				Name:      m.opensearchCluster.Name,
				Namespace: m.storageNamespace,
			}, cluster)
			if err != nil {
				m.logger.Error("failed to fetch opensearch cluster, can't check readiness")
				return true
			}
			if !cluster.Status.Initialized {
				continue
			}
			break FETCH
		}
	}
	return false
}

func (m *LoggingManagerV2) setOpensearchClient() *opensearch.Client {
	ctx := context.TODO()
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
			m.logger.Warn("plugin context cancelled before Opensearch object created")
			return nil
		case <-b.Next():
			err := m.k8sClient.Get(ctx, types.NamespacedName{
				Name:      m.opensearchCluster.Name,
				Namespace: m.storageNamespace,
			}, cluster)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					m.logger.Info("waiting for k8s object")
					continue
				}
				m.logger.Errorf("failed to check k8s object: %v", err)
				continue
			}
			break FETCH
		}
	}

	username, _, err := helpers.UsernameAndPassword(ctx, m.k8sClient, cluster)
	if err != nil {
		m.logger.Errorf("failed to get cluster details: %v", err)
		panic(err)
	}

	certMgr := certs.NewCertMgrOpensearchCertManager(
		context.TODO(),
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
		m.logger.Errorf("failed to create client: %v", err)
		panic(err)
	}

	return client
}

func (m *LoggingManagerV2) generateAdminPassword(cluster *loggingv1beta1.OpniOpensearch) (password []byte, retErr error) {
	ctx := context.TODO()
	password = util.GenerateRandomString(8)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-user-password",
			Namespace: m.storageNamespace,
		},
		Data: map[string][]byte{
			"password": password,
		},
	}
	ctrl.SetControllerReference(cluster, secret, m.k8sClient.Scheme())
	retErr = m.k8sClient.Create(ctx, secret)
	if retErr != nil {
		if !k8serrors.IsAlreadyExists(retErr) {
			return
		}
		retErr = m.k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if retErr != nil {
			return
		}
		password = secret.Data["password"]
		return
	}

	return
}

func (m *LoggingManagerV2) createInitialAdmin() error {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}
	ctx := context.TODO()

	err := m.k8sClient.Get(ctx, types.NamespacedName{
		Name:      m.opensearchCluster.Name,
		Namespace: m.storageNamespace,
	}, k8sOpensearchCluster)
	if err != nil {
		return err
	}

	password, err := m.generateAdminPassword(k8sOpensearchCluster)
	if err != nil {
		return err
	}

	go m.opensearchManager.CreateInitialAdmin(password, m.opensearchClusterReady)
	return nil
}

func (m *LoggingManagerV2) convertProtobufToDashboards(
	dashboard *loggingadmin.DashboardsDetails,
	cluster *loggingv1beta1.OpniOpensearch,
) opsterv1.DashboardsConfig {
	var osVersion string
	version := "0.9.1-rc2"
	if cluster == nil {
		osVersion = opensearchVersion
	} else {
		if cluster.Status.Version != nil {
			version = *cluster.Status.Version
		} else {
			version = strings.TrimPrefix(versions.Version, "v")
		}
		if cluster.Status.OpensearchVersion != nil {
			osVersion = *cluster.Status.OpensearchVersion
		} else {
			osVersion = opensearchVersion
		}
	}

	if version == "unversioned" {
		version = "0.9.1-rc2"
	}

	if m.versionOverride != "" {
		version = m.versionOverride
	}

	image := fmt.Sprintf(
		"%s/opensearch-dashboards:%s-%s",
		defaultRepo,
		osVersion,
		version,
	)

	resources := corev1.ResourceRequirements{
		Requests: func() corev1.ResourceList {
			if dashboard.Resources == nil {
				return nil
			}
			if dashboard.Resources.Requests == nil {
				return nil
			}
			list := corev1.ResourceList{}
			if dashboard.Resources.Requests.Cpu != "" {
				list[corev1.ResourceCPU] = resource.MustParse(dashboard.Resources.Requests.Cpu)
			}
			if dashboard.Resources.Requests.Memory != "" {
				list[corev1.ResourceMemory] = resource.MustParse(dashboard.Resources.Requests.Memory)
			}
			return list
		}(),
		Limits: func() corev1.ResourceList {
			if dashboard.Resources == nil {
				return nil
			}
			if dashboard.Resources.Limits == nil {
				return nil
			}
			list := corev1.ResourceList{}
			if dashboard.Resources.Limits.Cpu != "" {
				list[corev1.ResourceCPU] = resource.MustParse(dashboard.Resources.Limits.Cpu)
			}
			if dashboard.Resources.Limits.Memory != "" {
				list[corev1.ResourceMemory] = resource.MustParse(dashboard.Resources.Limits.Memory)
			}
			return list
		}(),
	}

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
			return resources
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

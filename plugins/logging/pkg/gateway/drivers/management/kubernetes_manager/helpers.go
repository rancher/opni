package kubernetes_manager

import (
	"context"
	"fmt"
	"slices"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util"
	k8sutilerrors "github.com/rancher/opni/pkg/util/errors/k8sutil"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (d *KubernetesManagerDriver) generateNodePools(cluster *loggingadmin.OpensearchClusterV2) (pools []opsterv1.NodePool, retErr error) {
	resources, jvm, retErr := generateK8sResources(
		cluster.GetDataNodes().GetMemoryLimit(),
		cluster.GetDataNodes().GetCpuResources(),
	)
	if retErr != nil {
		return
	}

	if lo.FromPtrOr(cluster.DataNodes.Replicas, 1) < 1 {
		retErr = errors.ErrReplicasZero
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
				return d.generateAntiAffinity("data")
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
					return d.generateAntiAffinity("data")
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
		ingest, err := d.convertIngestDetails(cluster.IngestNodes)
		if err != nil {
			retErr = err
			return
		}
		pools = append(pools, ingest)
	}

	if cluster.GetControlplaneNodes() != nil {
		cp, err := d.convertControlplaneDetails(cluster.ControlplaneNodes)
		if err != nil {
			retErr = err
			return
		}
		pools = append(pools, cp)
	}

	return
}

func (d *KubernetesManagerDriver) generateAntiAffinity(opniRole string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								LabelOpsterCluster: d.OpensearchCluster.Name,
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

func (d *KubernetesManagerDriver) convertIngestDetails(details *loggingadmin.IngestDetails) (opsterv1.NodePool, error) {
	resources, jvm, err := generateK8sResources(details.MemoryLimit, details.CpuResources)
	if err != nil {
		return opsterv1.NodePool{}, err
	}

	if lo.FromPtrOr(details.Replicas, 1) < 1 {
		return opsterv1.NodePool{}, errors.ErrReplicasZero
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
				return d.generateAntiAffinity("ingest")
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

func (d *KubernetesManagerDriver) convertControlplaneDetails(details *loggingadmin.ControlplaneDetails) (opsterv1.NodePool, error) {
	_, jvm, err := generateK8sResources("512Mi", &loggingadmin.CPUResource{
		Request: "100m",
	})
	if err != nil {
		return opsterv1.NodePool{}, err
	}

	if lo.FromPtrOr(details.Replicas, 1) < 1 {
		return opsterv1.NodePool{}, errors.ErrReplicasZero
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
		Affinity:     d.generateAntiAffinity("controlplane"),
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

func convertProtobufToDashboards(
	dashboard *loggingadmin.DashboardsDetails,
	cluster *loggingv1beta1.OpniOpensearch,
	opniVersion string,
) opsterv1.DashboardsConfig {
	var osVersion string
	var version string
	if cluster == nil {
		osVersion = opensearchVersion
		version = opniVersion
	} else {
		if cluster.Status.Version != nil {
			version = *cluster.Status.Version
		} else {
			version = opniVersion
		}
		if cluster.Status.OpensearchVersion != nil {
			osVersion = *cluster.Status.OpensearchVersion
		} else {
			osVersion = opensearchVersion
		}
	}

	image := fmt.Sprintf(
		"%s/opensearch-dashboards:v%s-%s",
		defaultRepo,
		version,
		osVersion,
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
		Service: opsterv1.DashboardsServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
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

func (d *KubernetesManagerDriver) storeS3Credentials(ctx context.Context, credentials *loggingadmin.S3Credentials) error {
	if credentials == nil {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s3CredentialsSecret,
			Namespace: d.OpensearchCluster.Namespace,
		},
	}

	err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			secret.StringData = map[string]string{
				"accessKey": credentials.GetAccessKey(),
				"secretKey": credentials.GetSecretKey(),
			}
			err = d.K8sClient.Create(ctx, secret)
			if err != nil {
				d.Logger.Error(fmt.Sprintf("failed to create s3 credentials secret: %v", err))
				return k8sutilerrors.GRPCFromK8s(err)
			}
			return nil
		}
		d.Logger.Error(fmt.Sprintf("failed to get existing s3 credentials: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if err != nil {
			return err
		}

		secret.Data["accessKey"] = []byte(credentials.GetAccessKey())
		if credentials.GetSecretKey() != "" {
			secret.Data["secretKey"] = []byte(credentials.GetSecretKey())
		}
		return d.K8sClient.Update(ctx, secret)
	})
	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update s3 credentials: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) getS3Credentials(ctx context.Context) (*loggingadmin.S3Credentials, error) {
	secret := &corev1.Secret{}
	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      s3CredentialsSecret,
		Namespace: d.OpensearchCluster.Namespace,
	}, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		d.Logger.Error(fmt.Sprintf("failed to get s3 credentials: %v", err))
		return nil, k8sutilerrors.GRPCFromK8s(err)
	}
	return &loggingadmin.S3Credentials{
		AccessKey: string(secret.Data["accessKey"]),
	}, nil
}

func (d *KubernetesManagerDriver) s3ToProtobuf(
	ctx context.Context,
	in *loggingv1beta1.OpensearchS3Settings,
) *loggingadmin.OpensearchS3Settings {
	if in == nil {
		return nil
	}

	creds, _ := d.getS3Credentials(ctx)

	return &loggingadmin.OpensearchS3Settings{
		Endpoint:        in.Endpoint,
		Insecure:        in.Protocol == loggingv1beta1.OpensearchS3ProtocolHTTP,
		PathStyleAccess: in.PathStyleAccess,
		Credentials:     creds,
		Bucket:          in.Repository.Bucket,
		Folder: func() *string {
			if in.Repository.Folder == "" {
				return nil
			}
			return lo.ToPtr(in.Repository.Folder)
		}(),
		ProxySettings: func() *loggingadmin.ProxySettings {
			if in.ProxyHost != "" && in.ProxyPort != nil {
				return &loggingadmin.ProxySettings{
					ProxyHost: in.ProxyHost,
					ProxyPort: in.ProxyPort,
				}
			}
			return nil
		}(),
	}
}

func (d *KubernetesManagerDriver) createOrUpdateRecurringSnapshot(
	ctx context.Context,
	snapshot *loggingadmin.SnapshotSchedule,
	defaultIndices []string,
	owner metav1.Object,
) error {
	k8sSnapshot := &loggingv1beta1.RecurringSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshot.GetRef().GetName(),
			Namespace: d.OpensearchCluster.Namespace,
		},
	}

	err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sSnapshot), k8sSnapshot)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			d.Logger.Error(fmt.Sprintf("failed to check if snapshot exists: %v", err))
			return k8sutilerrors.GRPCFromK8s(err)
		}
		controllerutil.SetOwnerReference(owner, k8sSnapshot, d.K8sClient.Scheme())
		d.updateRecurringSnapshot(k8sSnapshot, snapshot, defaultIndices)
		err = d.K8sClient.Create(ctx, k8sSnapshot)
		if err != nil {
			d.Logger.Error(fmt.Sprintf("failed to create snapshot: %v", err))
			return k8sutilerrors.GRPCFromK8s(err)
		}
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sSnapshot), k8sSnapshot)
		if err != nil {
			return err
		}
		d.updateRecurringSnapshot(k8sSnapshot, snapshot, defaultIndices)
		return d.K8sClient.Update(ctx, k8sSnapshot)
	})

	if err != nil {
		d.Logger.Error(fmt.Sprintf("failed to update snapshot: %v", err))
		return k8sutilerrors.GRPCFromK8s(err)
	}
	return nil
}

func (d *KubernetesManagerDriver) updateRecurringSnapshot(
	k8sSnapshot *loggingv1beta1.RecurringSnapshot,
	snapshot *loggingadmin.SnapshotSchedule,
	defaultIndices []string,
) {
	k8sSnapshot.Spec = loggingv1beta1.RecurringSnapshotSpec{
		Snapshot: loggingv1beta1.SnapshotSpec{
			Indices: append(snapshot.GetAdditionalIndices(), defaultIndices...),
			Repository: corev1.LocalObjectReference{
				Name: d.OpensearchCluster.Name,
			},
		},
		Creation: loggingv1beta1.RecurringSnapshotCreation{
			CronSchedule: snapshot.GetCronSchedule(),
		},
		Retention: func() *loggingv1beta1.RecurringSnapshotRetention {
			if snapshot.Retention == nil {
				return nil
			}

			return &loggingv1beta1.RecurringSnapshotRetention{
				MaxAge:   snapshot.Retention.GetTimeRetention(),
				MaxCount: snapshot.Retention.MaxSnapshots,
			}
		}(),
	}
}

func (d *KubernetesManagerDriver) listRecurringSnapshots(ctx context.Context) (retSlice []*loggingadmin.SnapshotStatus, retErr error) {
	list := &loggingv1beta1.RecurringSnapshotList{}
	retErr = d.K8sClient.List(ctx, list, client.InNamespace(d.OpensearchCluster.Namespace))
	if retErr != nil {
		d.Logger.Error(fmt.Sprintf("failed to list recurring snapshots: %v", retErr))
		retErr = k8sutilerrors.GRPCFromK8s(retErr)
		return
	}

	for _, s := range list.Items {
		retSlice = append(retSlice, &loggingadmin.SnapshotStatus{
			Ref: &loggingadmin.SnapshotReference{
				Name: s.Name,
			},
			Status: func() string {
				if s.Status.ExecutionStatus == nil {
					return ""
				}
				return string(s.Status.ExecutionStatus.Status)
			}(),
			StatusMessage: func() *string {
				if s.Status.ExecutionStatus == nil {
					return nil
				}
				if s.Status.ExecutionStatus.Cause != "" {
					return &s.Status.ExecutionStatus.Cause
				}
				if s.Status.ExecutionStatus.Message != "" {
					return &s.Status.ExecutionStatus.Message
				}
				return nil
			}(),
			LastUpdated: func() *timestamppb.Timestamp {
				if s.Status.ExecutionStatus == nil {
					return timestamppb.New(s.CreationTimestamp.Time)
				}
				return timestamppb.New(s.Status.ExecutionStatus.LastExecution.Time)
			}(),
			Recurring: true,
		})
	}
	return
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

	return persistence, errors.ErrInvalidPersistence
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

func generateK8sResources(memoryLimit string, cpu *loggingadmin.CPUResource) (corev1.ResourceRequirements, string, error) {
	if memoryLimit == "" {
		return corev1.ResourceRequirements{}, "", errors.ErrRequestMissingMemory
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
				return corev1.ResourceRequirements{}, "", errors.ErrRequestGtLimits
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

func convertDashboardsToProtobuf(dashboard opsterv1.DashboardsConfig) *loggingadmin.DashboardsDetails {
	return &loggingadmin.DashboardsDetails{
		Enabled:  &dashboard.Enable,
		Replicas: &dashboard.Replicas,
		Resources: func() *loggingadmin.ResourceRequirements {
			if dashboard.Resources.Limits == nil && dashboard.Resources.Requests == nil {
				return nil
			}
			resources := &loggingadmin.ResourceRequirements{}
			if dashboard.Resources.Requests != nil {
				resources.Requests = &loggingadmin.ComputeResourceQuantities{
					Cpu: func() string {
						if dashboard.Resources.Requests.Cpu().IsZero() {
							return ""
						}
						return dashboard.Resources.Requests.Cpu().String()
					}(),
					Memory: func() string {
						if dashboard.Resources.Requests.Memory().IsZero() {
							return ""
						}
						return dashboard.Resources.Requests.Memory().String()
					}(),
				}
			}
			if dashboard.Resources.Limits != nil {
				resources.Requests = &loggingadmin.ComputeResourceQuantities{
					Cpu: func() string {
						if dashboard.Resources.Limits.Cpu().IsZero() {
							return ""
						}
						return dashboard.Resources.Limits.Cpu().String()
					}(),
					Memory: func() string {
						if dashboard.Resources.Limits.Memory().IsZero() {
							return ""
						}
						return dashboard.Resources.Limits.Memory().String()
					}(),
				}
			}

			return resources
		}(),
	}
}

func s3ToKubernetes(in *loggingadmin.OpensearchS3Settings) *loggingv1beta1.OpensearchS3Settings {
	if in == nil {
		return nil
	}

	return &loggingv1beta1.OpensearchS3Settings{
		Endpoint:        in.Endpoint,
		PathStyleAccess: in.PathStyleAccess,
		Protocol: func() loggingv1beta1.OpensearchS3Protocol {
			if in.Insecure {
				return loggingv1beta1.OpensearchS3ProtocolHTTP
			}
			return loggingv1beta1.OpensearchS3ProtocolHTTPS
		}(),
		ProxyHost: in.GetProxySettings().GetProxyHost(),
		ProxyPort: func() *int32 {
			if in.GetProxySettings() == nil {
				return nil
			}
			return in.GetProxySettings().ProxyPort
		}(),
		CredentialSecret: corev1.LocalObjectReference{
			Name: s3CredentialsSecret,
		},
		Repository: loggingv1beta1.S3PathSettings{
			Bucket: in.Bucket,
			Folder: in.GetFolder(),
		},
	}
}

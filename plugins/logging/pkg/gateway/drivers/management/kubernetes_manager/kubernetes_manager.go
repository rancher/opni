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
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
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

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions
}

type KubernetesManagerDriverOptions struct {
	OpensearchCluster *opnimeta.OpensearchClusterRef `option:"opensearchCluster"`
	K8sClient         client.Client                  `option:"k8sClient"`
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
		return
	}

	password = util.GenerateRandomString(8)
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
			return
		}
		retErr = d.K8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if retErr != nil {
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
		return err
	}

	if len(loggingClusters.Items) > 0 {
		return errors.ErrLoggingCapabilityExists
	}

	cluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.OpensearchCluster.Name,
			Namespace: d.OpensearchCluster.Namespace,
		},
	}

	return d.K8sClient.Delete(ctx, cluster)
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

func (d *KubernetesManagerDriver) CreateOrUpdateCluster(
	ctx context.Context,
	cluster *loggingadmin.OpensearchClusterV2,
	opniVersion string,
	natName string,
) error {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}
	exists := true
	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, k8sOpensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
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
					Dashboards: d.convertProtobufToDashboards(cluster.Dashboards, nil, opniVersion),
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
				Version:           opniVersion,
				ImageRepo:         "docker.io/rancher",
				NatsRef: &corev1.LocalObjectReference{
					Name: natName,
				},
			},
		}

		return d.K8sClient.Create(ctx, k8sOpensearchCluster)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.OpensearchSettings.NodePools = nodePools
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards = d.convertProtobufToDashboards(
			cluster.Dashboards,
			k8sOpensearchCluster,
			opniVersion,
		)
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
}

func (d *KubernetesManagerDriver) UpgradeAvailable(ctx context.Context, opniVersion string) (bool, error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}

	err := d.K8sClient.Get(ctx, types.NamespacedName{
		Name:      d.OpensearchCluster.Name,
		Namespace: d.OpensearchCluster.Namespace,
	}, k8sOpensearchCluster)
	if err != nil {
		return false, err
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

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := d.K8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}

		k8sOpensearchCluster.Spec.Version = opniVersion
		k8sOpensearchCluster.Spec.OpensearchVersion = opensearchVersion

		image := fmt.Sprintf(
			"%s/opensearch-dashboards:%s-%s",
			defaultRepo,
			opensearchVersion,
			opniVersion,
		)

		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Image = &image
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards.Version = opensearchVersion

		return d.K8sClient.Update(ctx, k8sOpensearchCluster)
	})
}

func (d *KubernetesManagerDriver) GetStorageClasses(ctx context.Context) ([]string, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := d.K8sClient.List(ctx, storageClasses); err != nil {
		return nil, err
	}

	storageClassNames := make([]string, 0, len(storageClasses.Items))
	for _, storageClass := range storageClasses.Items {
		storageClassNames = append(storageClassNames, storageClass.Name)
	}

	return storageClassNames, nil
}

func (d *KubernetesManagerDriver) generateNodePools(cluster *loggingadmin.OpensearchClusterV2) (pools []opsterv1.NodePool, retErr error) {
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

func (d *KubernetesManagerDriver) convertProtobufToDashboards(
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

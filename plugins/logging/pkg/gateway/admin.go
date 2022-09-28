package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/opensearch-project/opensearch-go"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/samber/lo"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelOpsterCluster  = "opster.io/opensearch-cluster"
	LabelOpsterNodePool = "opster.io/opensearch-nodepool"
	TopologyKeyK8sHost  = "kubernetes.io/hostname"

	opensearchVersion = "1.3.3"
	defaultRepo       = "docker.io/rancher"
)

func (p *Plugin) GetOpensearchCluster(
	ctx context.Context,
	empty *emptypb.Empty,
) (*loggingadmin.OpensearchCluster, error) {
	cluster := &loggingv1beta1.OpniOpensearch{}
	if err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      p.opensearchCluster.Name,
		Namespace: p.opensearchCluster.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			p.logger.Info("opensearch cluster does not exist")
			return &loggingadmin.OpensearchCluster{}, nil
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
	// Check that it is safe to delete the cluster
	loggingClusters := &opnicorev1beta1.LoggingClusterList{}
	err := p.k8sClient.List(p.ctx, loggingClusters, client.InNamespace(p.storageNamespace))
	if err != nil {
		return nil, err
	}

	if len(loggingClusters.Items) > 0 {
		p.logger.Error("can not delete opensearch until logging capability is uninstalled from all clusters")
		return nil, errors.ErrLoggingCapabilityExists
	}

	cluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.opensearchCluster.Name,
			Namespace: p.storageNamespace,
		},
	}
	return &emptypb.Empty{}, p.k8sClient.Delete(ctx, cluster)
}

func (p *Plugin) CreateOrUpdateOpensearchCluster(
	ctx context.Context,
	cluster *loggingadmin.OpensearchCluster,
) (*emptypb.Empty, error) {
	// Validate retention string
	if !p.validDurationString(lo.FromPtrOr(cluster.DataRetention, "7d")) {
		return &emptypb.Empty{}, ErrInvalidRetention()
	}
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}

	exists := true
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      p.opensearchCluster.Name,
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
		convertedPool, err := convertProtobufToNodePool(pool, p.opensearchCluster.Name)
		if err != nil {
			return nil, err
		}
		nodePools = append(nodePools, convertedPool)
	}

	if !exists {
		k8sOpensearchCluster = &loggingv1beta1.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.opensearchCluster.Name,
				Namespace: p.storageNamespace,
			},
			Spec: loggingv1beta1.OpniOpensearchSpec{
				OpensearchSettings: loggingv1beta1.OpensearchSettings{
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
				ClusterConfigSpec: &loggingv1beta1.ClusterConfigSpec{
					IndexRetention: lo.FromPtrOr(cluster.DataRetention, "7d"),
				},
				OpensearchVersion: opensearchVersion,
				Version: func() string {
					if p.version != "" {
						return p.version
					}
					return util.Version
				}(),
				ImageRepo: "docker.io/rancher",
			},
		}

		err = p.k8sClient.Create(ctx, k8sOpensearchCluster)
		if err != nil {
			return nil, err
		}
		go p.setOpensearchClient()
		return &emptypb.Empty{}, nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := p.k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOpensearchCluster), k8sOpensearchCluster); err != nil {
			return err
		}
		k8sOpensearchCluster.Spec.OpensearchSettings.NodePools = nodePools
		k8sOpensearchCluster.Spec.OpensearchSettings.Dashboards = p.convertProtobufToDashboards(cluster.Dashboards, k8sOpensearchCluster)
		k8sOpensearchCluster.Spec.ExternalURL = cluster.ExternalURL
		if cluster.DataRetention != nil {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = &loggingv1beta1.ClusterConfigSpec{
				IndexRetention: *cluster.DataRetention,
			}
		} else {
			k8sOpensearchCluster.Spec.ClusterConfigSpec = nil
		}

		return p.k8sClient.Update(ctx, k8sOpensearchCluster)
	})

	return &emptypb.Empty{}, err
}

func (p *Plugin) UpgradeAvailable(context.Context, *emptypb.Empty) (*loggingadmin.UpgradeAvailableResponse, error) {
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{}
	var version string
	version = util.Version
	if p.version != "" {
		version = p.version
	}

	err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      p.opensearchCluster.Name,
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
	k8sOpensearchCluster := &loggingv1beta1.OpniOpensearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.opensearchCluster.Name,
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

	return &emptypb.Empty{}, err
}

func (p *Plugin) GetStorageClasses(ctx context.Context, in *emptypb.Empty) (*loggingadmin.StorageClassResponse, error) {
	storageClasses := &storagev1.StorageClassList{}
	if err := p.k8sClient.List(ctx, storageClasses); err != nil {
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

func convertNodePoolToProtobuf(pool opsterv1.NodePool) (*loggingadmin.OpensearchNodeDetails, error) {
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
				return &loggingadmin.OpensearchNodeDetails{}, errors.ErrStoredClusterPersistence()
			}
		}
	}

	return &loggingadmin.OpensearchNodeDetails{
		Name:        pool.Component,
		Replicas:    &pool.Replicas,
		DiskSize:    pool.DiskSize,
		MemoryLimit: pool.Resources.Limits.Memory().String(),
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
				Request: pool.Resources.Requests.Cpu().String(),
				Limit:   pool.Resources.Limits.Cpu().String(),
			}
		}(),
		NodeSelector: pool.NodeSelector,
		Tolerations:  tolerations,
		Persistence:  &persistence,
		Roles:        replaceInArray(pool.Roles, "master", "controlplane"),
		EnableAntiAffinity: func() *bool {
			if pool.Affinity == nil {
				return lo.ToPtr(false)
			}
			enabled := pool.Affinity.PodAntiAffinity != nil
			return &enabled
		}(),
	}, nil
}

func (p *Plugin) convertProtobufToDashboards(
	dashboard *loggingadmin.DashboardsDetails,
	cluster *loggingv1beta1.OpniOpensearch,
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

	resources := corev1.ResourceRequirements{
		Requests: func() corev1.ResourceList {
			if dashboard.Resources == nil {
				return nil
			}
			if dashboard.Resources.Requests == nil {
				return nil
			}
			list := corev1.ResourceList{}
			if dashboard.Resources.Requests.CPU != "" {
				list[corev1.ResourceCPU] = resource.MustParse(dashboard.Resources.Requests.CPU)
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
			if dashboard.Resources.Limits.CPU != "" {
				list[corev1.ResourceCPU] = resource.MustParse(dashboard.Resources.Limits.CPU)
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

func (p *Plugin) setOpensearchClient() {
	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(p.ctx)

	cluster := &opsterv1.OpenSearchCluster{}

FETCH:
	for {
		select {
		case <-b.Done():
			p.logger.Warn("plugin context cancelled before Opensearch object created")
		case <-b.Next():
			err := p.k8sClient.Get(p.ctx, types.NamespacedName{
				Name:      p.opensearchCluster.Name,
				Namespace: p.storageNamespace,
			}, cluster)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					p.logger.Info("waiting for k8s object")
					continue
				}
				p.logger.Errorf("failed to check k8s object: %v", err)
				continue
			}
			break FETCH
		}
	}

	username, password, err := helpers.UsernameAndPassword(p.ctx, p.k8sClient, cluster)
	if err != nil {
		p.logger.Errorf("failed to get cluster details: %v", err)
		return
	}

	// Set sane transport timeouts
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout: 5 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	osCfg := opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://%s.%s:9200", cluster.Spec.General.ServiceName, cluster.Namespace),
		},
		Username:             username,
		Password:             password,
		UseResponseCheckOnly: true,
		Transport:            transport,
	}

	osclient, err := opensearch.NewClient(osCfg)
	if err != nil {
		p.logger.Errorf("failed to create opensearch client: %v", err)
		return
	}

	p.opensearchClient.Set(osclient)
}

func (p *Plugin) validDurationString(duration string) bool {
	match, err := regexp.MatchString(`^\d+[dMmyh]`, duration)
	if err != nil {
		p.logger.Errorf("could not run regexp: %v", err)
		return false
	}
	return match
}

func convertProtobufToNodePool(pool *loggingadmin.OpensearchNodeDetails, clusterName string) (opsterv1.NodePool, error) {
	if pool.MemoryLimit == "" {
		return opsterv1.NodePool{}, errors.ErrRequestMissingMemory()
	}

	memory, err := resource.ParseQuantity(pool.MemoryLimit)
	if err != nil {
		return opsterv1.NodePool{}, err
	}
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: memory,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: memory,
		},
	}

	if pool.CPUResources != nil {
		if pool.CPUResources.Request != "" {
			request, err := resource.ParseQuantity(pool.CPUResources.Request)
			if err != nil {
				return opsterv1.NodePool{}, err
			}
			resources.Requests[corev1.ResourceCPU] = request
		}
		if pool.CPUResources.Limit != "" {
			limit, err := resource.ParseQuantity(pool.CPUResources.Limit)
			if err != nil {
				return opsterv1.NodePool{}, err
			}
			resources.Limits[corev1.ResourceCPU] = limit
		}
	}

	jvmVal := memory.Value() / 2

	return opsterv1.NodePool{
		Component: pool.Name,
		Replicas:  lo.FromPtrOr(pool.Replicas, 1),
		DiskSize:  pool.DiskSize,
		Resources: resources,
		Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", jvmVal, jvmVal),
		Roles:     replaceInArray(pool.Roles, "controlplane", "master"),
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
		Resources: func() *loggingadmin.ResourceRequirements {
			if dashboard.Resources.Limits == nil && dashboard.Resources.Requests == nil {
				return nil
			}
			resources := &loggingadmin.ResourceRequirements{}
			if dashboard.Resources.Requests != nil {
				resources.Requests = &loggingadmin.ComputeResourceQuantities{
					CPU: func() string {
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
					CPU: func() string {
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

func replaceInArray[T comparable](array []T, old T, new T) []T {
	newArray := make([]T, 0, len(array))
	for _, item := range array {
		if item == old {
			newArray = append(newArray, new)
		} else {
			newArray = append(newArray, item)
		}
	}
	return newArray
}

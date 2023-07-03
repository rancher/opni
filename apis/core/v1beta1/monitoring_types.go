package v1beta1

import (
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	"github.com/rancher/opni/internal/cortex/config/compactor"
	"github.com/rancher/opni/internal/cortex/config/querier"
	"github.com/rancher/opni/internal/cortex/config/runtimeconfig"
	"github.com/rancher/opni/internal/cortex/config/validation"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v1beta2 spec
type AlertingSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:default=9093
	WebPort int `json:"webPort,omitempty"`
	//+kubebuilder:default=9094
	ClusterPort int `json:"clusterPort,omitempty"`
	//+kubebuilder:default="ClusterIP"
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	//+kubebuilder:default="500Mi"
	Storage string `json:"storage,omitempty"`
	//+kubebuilder:default="500m"
	CPU string `json:"cpu,omitempty"`
	//+kubebuilder:default="200Mi"
	Memory string `json:"memory,omitempty"`
	//+kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`
	//+kubebuilder:default="1m0s"
	ClusterSettleTimeout string `json:"clusterSettleTimeout,omitempty"`
	//+kubebuilder:default="1m0s"
	ClusterPushPullInterval string `json:"clusterPushPullInterval,omitempty"`
	//+kubebuilder:default="200ms"
	ClusterGossipInterval string `json:"clusterGossipInterval,omitempty"`
	ConfigName            string `json:"configName,omitempty"`
	//+kubebuilder:default="/var/lib"
	DataMountPath       string                      `json:"dataMountPath,omitempty"`
	GatewayVolumeMounts []opnimeta.ExtraVolumeMount `json:"alertVolumeMounts,omitempty"`
	//! deprecated
	RawAlertManagerConfig string `json:"rawConfigMap,omitempty"`
	//! deprecated
	RawInternalRouting string `json:"rawInternalRouting,omitempty"`
}

type DeploymentMode string

const (
	DeploymentModeAllInOne        DeploymentMode = "AllInOne"
	DeploymentModeHighlyAvailable DeploymentMode = "HighlyAvailable"
)

type CortexSpec struct {
	Enabled        bool                   `json:"enabled,omitempty"`
	Image          *opnimeta.ImageSpec    `json:"image,omitempty"`
	LogLevel       string                 `json:"logLevel,omitempty"`
	Storage        *storagev1.StorageSpec `json:"storage,omitempty"`
	ExtraEnvVars   []corev1.EnvVar        `json:"extraEnvVars,omitempty"`
	DeploymentMode DeploymentMode         `json:"deploymentMode,omitempty"`

	// Overrides for specific workloads. If unset, all values have automatic
	// defaults based on the deployment mode.
	Workloads       CortexWorkloadsSpec                `json:"workloads,omitempty"`
	Limits          *validation.Limits                 `json:"limits,omitempty"`
	RuntimeConfig   *runtimeconfig.RuntimeConfigValues `json:"runtimeConfig,omitempty"`
	CompactorConfig *compactor.Config                  `json:"compactorConfig,omitempty"`
	QuerierConfig   *querier.Config                    `json:"querierConfig,omitempty"`
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetStorage() *storagev1.StorageSpec {
	if cs == nil {
		return nil
	}
	return cs.Storage
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetLimits() *validation.Limits {
	if cs == nil {
		return nil
	}
	return cs.Limits
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetRuntimeConfig() *runtimeconfig.RuntimeConfigValues {
	if cs == nil {
		return nil
	}
	return cs.RuntimeConfig
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetCompactor() *compactor.Config {
	if cs == nil {
		return nil
	}
	return cs.CompactorConfig
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetQuerier() *querier.Config {
	if cs == nil {
		return nil
	}
	return cs.QuerierConfig
}

// Implements constraint configutil.cortexconfig
func (cs *CortexSpec) GetLogLevel() string {
	if cs == nil {
		return ""
	}
	return cs.LogLevel
}

type CortexWorkloadsSpec struct {
	Distributor   *CortexWorkloadSpec `json:"distributor,omitempty"`
	Ingester      *CortexWorkloadSpec `json:"ingester,omitempty"`
	Compactor     *CortexWorkloadSpec `json:"compactor,omitempty"`
	StoreGateway  *CortexWorkloadSpec `json:"storeGateway,omitempty"`
	Ruler         *CortexWorkloadSpec `json:"ruler,omitempty"`
	QueryFrontend *CortexWorkloadSpec `json:"queryFrontend,omitempty"`
	Querier       *CortexWorkloadSpec `json:"querier,omitempty"`
	Purger        *CortexWorkloadSpec `json:"purger,omitempty"`

	// Used only when deploymentMode is AllInOne.
	AllInOne *CortexWorkloadSpec `json:"allInOne,omitempty"`
}

type CortexWorkloadSpec struct {
	Replicas  *int32   `json:"replicas,omitempty"`
	ExtraArgs []string `json:"extraArgs,omitempty"`

	ExtraVolumes         []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts    []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars         []corev1.EnvVar                   `json:"extraEnvVars,omitempty"`
	SidecarContainers    []corev1.Container                `json:"sidecarContainers,omitempty"`
	InitContainers       []corev1.Container                `json:"initContainers,omitempty"`
	DeploymentStrategy   *appsv1.DeploymentStrategy        `json:"deploymentStrategy,omitempty"`
	UpdateStrategy       *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
	SecurityContext      *corev1.SecurityContext           `json:"securityContext,omitempty"`
	Affinity             *corev1.Affinity                  `json:"affinity,omitempty"`
	ResourceRequirements *corev1.ResourceRequirements      `json:"resourceLimits,omitempty"`
	NodeSelector         map[string]string                 `json:"nodeSelector,omitempty"`
	Tolerations          []corev1.Toleration               `json:"tolerations,omitempty"`
}

type GrafanaSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// Contains any additional configuration or overrides for the Grafana
	// installation spec.
	grafanav1alpha1.GrafanaSpec `json:",inline,omitempty"`
}

type MonitoringClusterSpec struct {
	//+kubebuilder:validation:Required
	Gateway corev1.LocalObjectReference `json:"gateway,omitempty"`
	Cortex  CortexSpec                  `json:"cortex,omitempty"`
	Grafana GrafanaSpec                 `json:"grafana,omitempty"`
}

type MonitoringClusterStatus struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Cortex          CortexStatus      `json:"cortex,omitempty"`
}

type CortexStatus struct {
	Version        string                    `json:"version,omitempty"`
	WorkloadsReady bool                      `json:"workloadsReady,omitempty"`
	Conditions     []string                  `json:"conditions,omitempty"`
	WorkloadStatus map[string]WorkloadStatus `json:"workloadStatus,omitempty"`
}

type WorkloadStatus struct {
	Ready   bool   `json:"ready,omitempty"`
	Message string `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MonitoringCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MonitoringClusterSpec   `json:"spec,omitempty"`
	Status            MonitoringClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MonitoringClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitoringCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&MonitoringCluster{}, &MonitoringClusterList{},
	)
}

package v1beta1

import (
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AlertingSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:default=9093
	WebPort int `json:"webPort,omitempty"`
	//+kubebuilder:default=9094
	ApiPort int `json:"apiPort,omitempty"`
	//+kubebuilder:default=ClusterIP
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	//+kubebuilder:default="500Mi"
	Storage string `json:"storage,omitempty"`
	//+kubebuilder:default="alertmanager-config"
	ConfigName          string                      `json:"configName,omitempty"`
	GatewayVolumeMounts []opnimeta.ExtraVolumeMount `json:"alertVolumeMounts,omitempty"`
}

type StorageBackendType string

const (
	StorageBackendS3         StorageBackendType = "s3"
	StorageBackendGCS        StorageBackendType = "gcs"
	StorageBackendAzure      StorageBackendType = "azure"
	StorageBackendSwift      StorageBackendType = "swift"
	StorageBackendFilesystem StorageBackendType = "filesystem"
)

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
	Workloads CortexWorkloadsSpec `json:"workloads,omitempty"`
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
	Replicas           *int32                            `json:"replicas,omitempty"`
	ExtraVolumes       []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts  []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars       []corev1.EnvVar                   `json:"extraEnvVars,omitempty"`
	ExtraArgs          []string                          `json:"extraArgs,omitempty"`
	SidecarContainers  []corev1.Container                `json:"sidecarContainers,omitempty"`
	InitContainers     []corev1.Container                `json:"initContainers,omitempty"`
	DeploymentStrategy *appsv1.DeploymentStrategy        `json:"deploymentStrategy,omitempty"`
	UpdateStrategy     *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
	SecurityContext    *corev1.SecurityContext           `json:"securityContext,omitempty"`
	Affinity           *corev1.Affinity                  `json:"affinity,omitempty"`
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
	Version string `json:"version,omitempty"`
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

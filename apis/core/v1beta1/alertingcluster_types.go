package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AlertingDeployConfStandalone = "standalone"
	AlertingDeployConfCluster    = "ha"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type AlertingCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertingClusterSpec   `json:"spec,omitempty"`
	Status            AlertingClusterStatus `json:"status,omitempty"`
}

type AlertingClusterSpec struct {
	//+kubebuilder:validation:required
	Gateway      corev1.LocalObjectReference `json:"gateway,omitempty"`
	Alertmanager AlertManagerSpec            `json:"alertmanager,omitempty"`
}

type AlertManagerSpec struct {
	Enable     bool                `json:"enable,omitempty"`
	Image      *opnimeta.ImageSpec `json:"image,omitempty"`
	LogLevel   string              `json:"logLevel,omitempty"`
	DeployConf string

	// Overrides for out-of-the box alerting specs
	ApplicationSpec AlertingApplicationSpec `json:"workloads,omitempty"`
}

type AlertingApplicationSpec struct {
	Replicas             *int32                            `json:"replicas,omitempty"`
	ExtraArgs            []string                          `json:"extraArgs,omitempty"`
	ExtraVolumes         []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts    []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars         []corev1.EnvVar                   `json:"extraEnvVars,omitempty"`
	SidecarContainers    []corev1.Container                `json:"sidecarContainers,omitempty"`
	ResourceRequirements *corev1.ResourceRequirements      `json:"resourceLimits,omitempty"`
	UpdateStrategy       *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
	SecurityContext      *corev1.SecurityContext           `json:"securityContext,omitempty"`
	Affinity             *corev1.Affinity                  `json:"affinity,omitempty"`
}

type AlertingClusterStatus struct {
	Image           string             `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy  `json:"imagePullPolicy,omitempty"`
	Alertmanager    AlertManagerStatus `json:"alertmanager,omitempty"`
}

type AlertManagerStatus struct {
	Version    string   `json:"version,omitempty"`
	Ready      bool     `json:"ready,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type AlertingClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AlertingCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AlertingCluster{}, &AlertingClusterList{})
}

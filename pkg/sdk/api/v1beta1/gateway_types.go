package v1beta1

import (
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/noauth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ImageSpec struct {
	Image            *string                       `json:"image,omitempty"`
	ImagePullPolicy  *corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (i *ImageSpec) ImageOrDefault(def string) string {
	if i == nil || i.Image == nil {
		return def
	}
	return *i.Image
}

func (i *ImageSpec) ImagePullPolicyOrDefault() corev1.PullPolicy {
	if i == nil || i.ImagePullPolicy == nil {
		return corev1.PullPolicy("")
	}
	return *i.ImagePullPolicy
}

type ExtraVolumeMount struct {
	Name         string              `json:"name,omitempty"`
	MountPath    string              `json:"mountPath,omitempty"`
	ReadOnly     bool                `json:"readOnly,omitempty"`
	VolumeSource corev1.VolumeSource `json:",inline"`
}

type GatewaySpec struct {
	Image            *ImageSpec             `json:"image,omitempty"`
	Auth             AuthSpec               `json:"auth,omitempty"`
	PluginSearchDirs []string               `json:"pluginSearchDirs,omitempty"`
	DNSNames         []string               `json:"dnsNames,omitempty"`
	ServiceType      string                 `json:"serviceType,omitempty"`
	Management       v1beta1.ManagementSpec `json:"management,omitempty"`

	NodeSelector      map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity          *corev1.Affinity    `json:"affinity,omitempty"`
	ExtraVolumeMounts []ExtraVolumeMount  `json:"extraVolumeMounts,omitempty"`
}

type AuthSpec struct {
	Provider string               `json:"provider,omitempty"`
	Openid   *openid.OpenidConfig `json:"openid,omitempty"`
	Noauth   *noauth.ServerConfig `json:"noauth,omitempty"`
}

//+kubebuilder:object:root=true
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GatewaySpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}

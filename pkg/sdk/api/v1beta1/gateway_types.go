package v1beta1

import (
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ImageSpec struct {
	Image            *string                       `json:"image,omitempty"`
	ImagePullPolicy  *corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (i *ImageSpec) GetImageWithDefault(def string) string {
	if i == nil || i.Image == nil {
		return def
	}
	return *i.Image
}

func (i *ImageSpec) GetImagePullPolicy() corev1.PullPolicy {
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
	//+kubebuilder:validation:Required
	Auth AuthSpec `json:"auth,omitempty"`

	Image            *ImageSpec             `json:"image,omitempty"`
	DNSNames         []string               `json:"dnsNames,omitempty"`
	PluginSearchDirs []string               `json:"pluginSearchDirs,omitempty"`
	ServiceType      corev1.ServiceType     `json:"serviceType,omitempty"`
	Management       v1beta1.ManagementSpec `json:"management,omitempty"`

	NodeSelector      map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity          *corev1.Affinity    `json:"affinity,omitempty"`
	ExtraVolumeMounts []ExtraVolumeMount  `json:"extraVolumeMounts,omitempty"`
}

func (g *GatewaySpec) GetServiceType() corev1.ServiceType {
	if g == nil || g.ServiceType == corev1.ServiceType("") {
		return corev1.ServiceTypeClusterIP
	}
	return g.ServiceType
}

type AuthSpec struct {
	//+kubebuilder:validation:Required
	Provider cfgv1beta1.AuthProviderType `json:"provider,omitempty"`
	Openid   *openid.OpenidConfig        `json:"openid,omitempty"`
	Noauth   *noauth.ServerConfig        `json:"noauth,omitempty"`
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

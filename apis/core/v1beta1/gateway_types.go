package v1beta1

import (
	"github.com/rancher/opni/pkg/auth/openid"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AuthSpec struct {
	//+kubebuilder:validation:Required
	Provider cfgv1beta1.AuthProviderType `json:"provider,omitempty"`
	Openid   *OpenIDConfigSpec           `json:"openid,omitempty"`
	Noauth   *noauth.ServerConfig        `json:"noauth,omitempty"`
}

type OpenIDConfigSpec struct {
	openid.OpenidConfig `json:",inline,omitempty,squash"`
	ClientID            string   `json:"clientID,omitempty"`
	ClientSecret        string   `json:"clientSecret,omitempty"`
	Scopes              []string `json:"scopes,omitempty"`
	AllowedDomains      []string `json:"allowedDomains,omitempty"`
	RoleAttributePath   string   `json:"roleAttributePath,omitempty"`

	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`

	// extra options from grafana config
	AllowSignUp         *bool  `json:"allowSignUp,omitempty"`
	RoleAttributeStrict *bool  `json:"roleAttributeStrict,omitempty"`
	EmailAttributePath  string `json:"emailAttributePath,omitempty"`
	TLSClientCert       string `json:"tlsClientCert,omitempty"`
	TLSClientKey        string `json:"tlsClientKey,omitempty"`
	TLSClientCA         string `json:"tlsClientCA,omitempty"`
}

type GatewaySpec struct {
	Image *opnimeta.ImageSpec `json:"image,omitempty"`
	//+kubebuilder:validation:Required
	Auth     AuthSpec `json:"auth,omitempty"`
	Hostname string   `json:"hostname,omitempty"`

	// Deprecated: this field is ignored.
	PluginSearchDirs []string `json:"pluginSearchDirs,omitempty"`

	Alerting AlertingSpec                `json:"alerting,omitempty"`
	NatsRef  corev1.LocalObjectReference `json:"natsCluster"`

	//+kubebuilder:default=LoadBalancer
	ServiceType        corev1.ServiceType        `json:"serviceType,omitempty"`
	ServiceAnnotations map[string]string         `json:"serviceAnnotations,omitempty"`
	Management         cfgv1beta1.ManagementSpec `json:"management,omitempty"`
	//+kubebuilder:default=jetstream
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	StorageType cfgv1beta1.StorageType `json:"storageType,omitempty"`

	NodeSelector      map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity          *corev1.Affinity            `json:"affinity,omitempty"`
	ExtraVolumeMounts []opnimeta.ExtraVolumeMount `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars      []corev1.EnvVar             `json:"extraEnvVars,omitempty"`
	Profiling         cfgv1beta1.ProfilingSpec    `json:"profiling,omitempty"`
}

func (g *GatewaySpec) GetServiceType() corev1.ServiceType {
	if g == nil || g.ServiceType == corev1.ServiceType("") {
		return corev1.ServiceTypeClusterIP
	}
	return g.ServiceType
}

type GatewayStatus struct {
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ServiceName     string                      `json:"serviceName,omitempty"`
	LoadBalancer    *corev1.LoadBalancerIngress `json:"loadBalancer,omitempty"`
	Endpoints       []corev1.EndpointAddress    `json:"endpoints,omitempty"`
	Ready           bool                        `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GatewaySpec   `json:"spec,omitempty"`
	Status            GatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&Gateway{}, &GatewayList{},
	)
}

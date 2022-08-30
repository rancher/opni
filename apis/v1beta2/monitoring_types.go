package v1beta2

import (
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
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

type GatewaySpec struct {
	Image *ImageSpec `json:"image,omitempty"`
	//+kubebuilder:validation:Required
	Auth             AuthSpec `json:"auth,omitempty"`
	Hostname         string   `json:"hostname,omitempty"`
	PluginSearchDirs []string `json:"pluginSearchDirs,omitempty"`

	Alerting *AlertingSpec `json:"alerting,omitempty"`

	//+kubebuilder:default=LoadBalancer
	ServiceType        corev1.ServiceType     `json:"serviceType,omitempty"`
	ServiceAnnotations map[string]string      `json:"serviceAnnotations,omitempty"`
	Management         v1beta1.ManagementSpec `json:"management,omitempty"`
	//+kubebuilder:default=etcd
	StorageType cfgv1beta1.StorageType `json:"storageType,omitempty"`

	NodeSelector      map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity          *corev1.Affinity            `json:"affinity,omitempty"`
	ExtraVolumeMounts []opnimeta.ExtraVolumeMount `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars      []corev1.EnvVar             `json:"extraEnvVars,omitempty"`
}

func (g *GatewaySpec) GetServiceType() corev1.ServiceType {
	if g == nil || g.ServiceType == corev1.ServiceType("") {
		return corev1.ServiceTypeClusterIP
	}
	return g.ServiceType
}

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

type DeploymentMode string

const (
	DeploymentModeAllInOne        DeploymentMode = "AllInOne"
	DeploymentModeHighlyAvailable DeploymentMode = "HighlyAvailable"
)

type CortexSpec struct {
	Enabled        bool                   `json:"enabled,omitempty"`
	Image          *ImageSpec             `json:"image,omitempty"`
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

type GatewayStatus struct {
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ServiceName     string                      `json:"serviceName,omitempty"`
	LoadBalancer    *corev1.LoadBalancerIngress `json:"loadBalancer,omitempty"`
	Endpoints       []corev1.EndpointAddress    `json:"endpoints,omitempty"`
	Ready           bool                        `json:"ready,omitempty"`
}

type CortexStatus struct {
	Version string `json:"version,omitempty"`
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
		&MonitoringCluster{}, &MonitoringClusterList{},
		&Gateway{}, &GatewayList{},
	)
}

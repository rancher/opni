package v1

import (
	configv1 "github.com/rancher/opni/pkg/config/v1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GatewaySpec struct {
	Replicas              *int32                      `json:"replicas,omitempty"`
	Image                 *opnimeta.ImageSpec         `json:"image,omitempty"`
	AgentImageTagOverride string                      `json:"agentImageTagOverride,omitempty"`
	NatsRef               corev1.LocalObjectReference `json:"natsCluster"`
	ServiceType           corev1.ServiceType          `json:"serviceType,omitempty"`
	ServiceAnnotations    map[string]string           `json:"serviceAnnotations,omitempty"`
	NodeSelector          map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations           []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity              *corev1.Affinity            `json:"affinity,omitempty"`
	ExtraVolumeMounts     []opnimeta.ExtraVolumeMount `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars          []corev1.EnvVar             `json:"extraEnvVars,omitempty"`

	//+kubebuilder:validation:Schemaless
	//+kubebuilder:pruning:PreserveUnknownFields
	//+kubebuilder:validation:Type=object
	Config *configv1.GatewayConfigSpec `json:"config,omitempty"`
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

type ValueStoreMethods struct{}

// ControllerReference implements crds.ValueStoreMethods.
func (m ValueStoreMethods) ControllerReference() (client.Object, bool) {
	return nil, false // Gateway is a top-level object
}

// FillConfigFromObject implements crds.ValueStoreMethods.
func (ValueStoreMethods) FillConfigFromObject(obj *Gateway, conf *configv1.GatewayConfigSpec) {
	proto.Merge(conf, obj.Spec.Config)
}

// FillObjectFromConfig implements crds.ValueStoreMethods.
func (ValueStoreMethods) FillObjectFromConfig(obj *Gateway, conf *configv1.GatewayConfigSpec) {
	obj.Spec.Config = conf
	obj.Spec.Config.Revision = nil
}

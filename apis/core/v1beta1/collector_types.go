package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CollectorState string

const (
	CollectorStatePending CollectorState = "pending"
	CollectorStateReady   CollectorState = "ready"
	CollectorStateError   CollectorState = "error"
)

type CollectorSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	AgentEndpoint      string                       `json:"agentEndpoint,omitempty"`
	SystemNamespace    string                       `json:"systemNamespace,omitempty"`
	LoggingConfig      *corev1.LocalObjectReference `json:"loggingConfig,omitempty"`
	MetricsConfig      *corev1.LocalObjectReference `json:"metricsConfig,omitempty"`
	ConfigReloader     *ConfigReloaderSpec          `json:"configReloader,omitempty"`
	LogLevel           string                       `json:"logLevel,omitempty"`
}

type ConfigReloaderSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
}

// CollectorStatus defines the observed state of Collector
type CollectorStatus struct {
	Conditions []string       `json:"conditions,omitempty"`
	State      CollectorState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion

// Collector is the Schema for the logadapters API
type Collector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CollectorSpec   `json:"spec,omitempty"`
	Status CollectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CollectorList contains a list of Collector
type CollectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Collector `json:"items"`
}

func CollectorCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(Collector{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:          GroupVersion.WithKind("Collector"),
		PluralName:   "collectors",
		Status:       true,
		Schema:       schema,
		NonNamespace: true,
	}, nil
}

func init() {
	SchemeBuilder.Register(&Collector{}, &CollectorList{})
}

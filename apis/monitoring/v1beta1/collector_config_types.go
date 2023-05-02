package v1beta1

import (
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PrometheusDiscovery struct {
	NamespaceSelector []string `json:"namespaceSelector,omitempty"`
}

type CollectorConfigSpec struct {
	PrometheusDiscovery PrometheusDiscovery `json:"prometheusDiscovery,omitempty"`
	RemoteWriteEndpoint string              `json:"remoteWriteEndpoint,omitempty"`
	OtelSpec            otel.OTELSpec       `json:"otelSpec,omitempty"`
}

type CollectorConfigStatus struct {
	Phase      string   `json:"phase,omitempty"`
	Message    string   `json:"message,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion

// CollectorConfig is the Schema for the monitoring API
type CollectorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CollectorConfigSpec   `json:"spec,omitempty"`
	Status CollectorConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CollectorConfigList contains a list of CollectorConfig
type CollectorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CollectorConfig `json:"items"`
}

func CollectorConfigCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(CollectorConfig{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:          GroupVersion.WithKind("CollectorConfig"),
		PluralName:   "collectorconfigs",
		Status:       true,
		Schema:       schema,
		NonNamespace: true,
	}, nil
}

func init() {
	SchemeBuilder.Register(&CollectorConfig{}, &CollectorConfigList{})
}

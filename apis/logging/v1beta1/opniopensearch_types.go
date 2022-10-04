package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	opsterv1 "opensearch.opster.io/api/v1"
)

type OpniOpensearchState string

const (
	OpniOpensearchStateError   OpniOpensearchState = "Error"
	OpniOpensearchStateWorking OpniOpensearchState = "Working"
	OpniOpensearchStateReady   OpniOpensearchState = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type OpniOpensearch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpniOpensearchSpec   `json:"spec,omitempty"`
	Status OpniOpensearchStatus `json:"status,omitempty"`
}

type OpniOpensearchStatus struct {
	Conditions        []string            `json:"conditions,omitempty"`
	State             OpniOpensearchState `json:"state,omitempty"`
	OpensearchVersion *string             `json:"opensearchVersion,omitempty"`
	Version           *string             `json:"version,omitempty"`
}

type OpniOpensearchSpec struct {
	*ClusterConfigSpec `json:",inline"`
	OpensearchSettings `json:"opensearch,omitempty"`
	ExternalURL        string                       `json:"externalURL,omitempty"`
	ImageRepo          string                       `json:"imageRepo"`
	OpensearchVersion  string                       `json:"opensearchVersion,omitempty"`
	Version            string                       `json:"version,omitempty"`
	NatsRef            *corev1.LocalObjectReference `json:"natsCluster"`
}

type OpensearchSettings struct {
	NodePools  []opsterv1.NodePool       `json:"nodePools,omitempty"`
	Dashboards opsterv1.DashboardsConfig `json:"dashboards,omitempty"`
	Security   *opsterv1.Security        `json:"security,omitempty"`
}

// +kubebuilder:object:root=true

// OpniOpensearchList contains a list of OpniOpensearch
type OpniOpensearchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpniOpensearch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpniOpensearch{}, &OpniOpensearchList{})
}

package v2beta1

import (
	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LoggingClusterState string

const (
	LoggingClusterStateError   LoggingClusterState = "Error"
	LoggingClusterStateWorking LoggingClusterState = "Working"
	LoggingClusterStateReady   LoggingClusterState = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type LoggingCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoggingClusterSpec   `json:"spec,omitempty"`
	Status LoggingClusterStatus `json:"status,omitempty"`
}

type LoggingClusterStatus struct {
	Conditions []string            `json:"conditions,omitempty"`
	State      LoggingClusterState `json:"state,omitempty"`
}

type LoggingClusterSpec struct {
	OpensearchCluster *opensearchv1beta1.OpensearchClusterRef `json:"opensearch,omitempty"`
}

// +kubebuilder:object:root=true

// LoggingClusterList contains a list of LoggingCluster
type LoggingClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoggingCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoggingCluster{}, &LoggingClusterList{})
}

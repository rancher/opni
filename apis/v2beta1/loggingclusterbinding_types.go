package v2beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type LoggingClusterBindingState string

const (
	LoggingClusterBindingStateError   LoggingClusterBindingState = "Error"
	LoggingClusterBindingStateWorking LoggingClusterBindingState = "Working"
	LoggingClusterBindingStateReady   LoggingClusterBindingState = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type LoggingClusterBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoggingClusterBindingSpec   `json:"spec,omitempty"`
	Status LoggingClusterBindingStatus `json:"status,omitempty"`
}

type LoggingClusterBindingSpec struct {
	DowmstreamClusters      []string              `json:"downstreamClusters,omitempty"`
	DownstreamLabelSelector *metav1.LabelSelector `json:"downstreamLabelSelector,omitempty"`
	OpensearchClusterRef    *OpensearchClusterRef `json:"opensearchClusterRef"`
	Username                string                `json:"username,omitempty"`
	Password                string                `json:"password,omitempty"`
}

type LoggingClusterBindingStatus struct {
	Conditions []string                   `json:"conditions,omitempty"`
	State      LoggingClusterBindingState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true

// LoggingClusterBindingList contains a list of LoggingClusterBinding
type LoggingClusterBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoggingClusterBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoggingClusterBinding{}, &LoggingClusterBindingList{})
}

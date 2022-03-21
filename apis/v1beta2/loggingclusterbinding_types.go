package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

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
	MulticlusterUser     *MulticlusterUserRef  `json:"user,omitempty"`
	LoggingCluster       *LoggingClusterRef    `json:"loggingCluster,omitempty"`
	OpensearchClusterRef *OpensearchClusterRef `json:"opensearchClusterRef"`
}

type MulticlusterUserRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type LoggingClusterRef struct {
	ID                   string                   `json:"id,omitempty"`
	LoggingClusterObject *LoggingClusterObjectRef `json:"loggingClusterName,omitempty"`
}

type LoggingClusterObjectRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
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

func (m *MulticlusterUserRef) ObjectKeyFromRef() types.NamespacedName {
	return types.NamespacedName{
		Name:      m.Name,
		Namespace: m.Namespace,
	}
}

func (l *LoggingClusterObjectRef) ObjectKeyFromRef() types.NamespacedName {
	return types.NamespacedName{
		Name:      l.Name,
		Namespace: l.Namespace,
	}
}

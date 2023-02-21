package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreprocessorState string

const (
	PreprocessorStatePending PreprocessorState = "pending"
	PreprocessorStateReady   PreprocessorState = "ready"
	PreprocessorStateError   PreprocessorState = "error"
)

type PreprocessorSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Replicas           *int                           `json:"replicas,omitempty"`
	OpensearchCluster  *opnimeta.OpensearchClusterRef `json:"opensearch,omitempty"`
}

// PreprocessorStatus defines the observed state of Preprocessor
type PreprocessorStatus struct {
	Conditions []string          `json:"conditions,omitempty"`
	State      PreprocessorState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:storageversion

// Preprocessor is the Schema for the logadapters API
type Preprocessor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreprocessorSpec   `json:"spec,omitempty"`
	Status PreprocessorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PreprocessorList contains a list of Preprocessor
type PreprocessorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Preprocessor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Preprocessor{}, &PreprocessorList{})
}

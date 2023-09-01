package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MulticlusterRoleBindingState string

const (
	MulticlusterRoleBindingStateError   MulticlusterRoleBindingState = "Error"
	MulticlusterRoleBindingStateWorking MulticlusterRoleBindingState = "Working"
	MulticlusterRoleBindingStateReady   MulticlusterRoleBindingState = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`
// +kubebuilder:storageversion

type MulticlusterRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MulticlusterRoleBindingSpec   `json:"spec,omitempty"`
	Status MulticlusterRoleBindingStatus `json:"status,omitempty"`
}

type MulticlusterRoleBindingStatus struct {
	Conditions []string                     `json:"conditions,omitempty"`
	State      MulticlusterRoleBindingState `json:"state,omitempty"`
}

type MulticlusterRoleBindingSpec struct {
	OpensearchCluster     *opnimeta.OpensearchClusterRef `json:"opensearch,omitempty"`
	OpensearchConfig      *ClusterConfigSpec             `json:"opensearchConfig,omitempty"`
	OpensearchExternalURL string                         `json:"opensearchExternalURL,omitempty"`
	NeuralSearchSettings  *opnimeta.NeuralSearchSettings `json:"neural_search,omitempty"`
}

type ClusterConfigSpec struct {
	IndexRetention string `json:"indexRetention,omitempty"`
}

// +kubebuilder:object:root=true

// MulticlusterRoleBindingList contains a list of MulticlusterRoleBinding
type MulticlusterRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MulticlusterRoleBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MulticlusterRoleBinding{}, &MulticlusterRoleBindingList{})
}

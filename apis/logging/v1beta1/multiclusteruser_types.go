package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MulticlusterUserState string

const (
	MulticlusterUserStateError   MulticlusterUserState = "Error"
	MulticlusterUserStatePending MulticlusterUserState = "Pending"
	MulticlusterUserStateCreated MulticlusterUserState = "Created"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`
// +kubebuilder:storageversion
type MulticlusterUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MulticlusterUserSpec   `json:"spec,omitempty"`
	Status MulticlusterUserStatus `json:"status,omitempty"`
}

type MulticlusterUserSpec struct {
	Password             string                         `json:"password,omitempty"`
	OpensearchClusterRef *opnimeta.OpensearchClusterRef `json:"opensearchClusterRef"`
}

type MulticlusterUserStatus struct {
	Conditions []string              `json:"conditions,omitempty"`
	State      MulticlusterUserState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true

// MulticlusterUserList contains a list of MulticlusterUser
type MulticlusterUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MulticlusterUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MulticlusterUser{}, &MulticlusterUserList{})
}

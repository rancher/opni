package v2beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IndexUserState string

const (
	IndexUserStatePending IndexUserState = "pending"
	IndexUserStateCreated IndexUserState = "created"
	IndexUserStateError   IndexUserState = "error"
)

const (
	IDLabel = "opni.io/multiclusterID"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="IndexUser",type=boolean,JSONPath=`.status.indexUserState`
type DownstreamCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DownstreamClusterSpec   `json:"spec,omitempty"`
	Status            DownstreamClusterStatus `json:"status,omitempty"`
}

type DownstreamClusterSpec struct {
	LoggingClusterRef *corev1.LocalObjectReference `json:"loggingCluster,omitempty"`
}

type DownstreamClusterStatus struct {
	Conditions         []string                     `json:"conditions,omitempty"`
	IndexUserState     IndexUserState               `json:"indexUserState,omitempty"`
	IndexUserSecretRef *corev1.LocalObjectReference `json:"indexUserSecret,omitempty"`
	ReadUserSecretRef  *corev1.LocalObjectReference `json:"readUserSecret,omitempty"`
}

//+kubebuilder:object:root=true
type DownstreamClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DownstreamCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DownstreamCluster{}, &DownstreamClusterList{})
}

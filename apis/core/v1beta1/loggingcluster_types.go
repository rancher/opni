package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IndexUserState string

const (
	IndexUserStatePending IndexUserState = "pending"
	IndexUserStateCreated IndexUserState = "created"
	IndexUserStateError   IndexUserState = "error"
)

type LoggingClusterState string

const (
	LoggingClusterStateCreated    LoggingClusterState = "created"
	LoggingClusterStateRegistered LoggingClusterState = "registered"
	LoggingClusterStateError      LoggingClusterState = "error"
)

const (
	IDLabel = "opni.io/multiclusterID"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="IndexUser",type=boolean,JSONPath=`.status.indexUserState`
type LoggingCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LoggingClusterSpec   `json:"spec,omitempty"`
	Status            LoggingClusterStatus `json:"status,omitempty"`
}

type LoggingClusterSpec struct {
	OpensearchClusterRef *opnimeta.OpensearchClusterRef `json:"opensearchCluster,omitempty"`
	// Deprecated field
	IndexUserSecret *corev1.LocalObjectReference `json:"indexUser,omitempty"`
	FriendlyName    string                       `json:"friendlyName,omitempty"`
	LastSync        metav1.Time                  `json:"lastSync,omitempty"`
	Enabled         bool                         `json:"enabled,omitempty"`
}

type LoggingClusterStatus struct {
	Conditions     []string            `json:"conditions,omitempty"`
	State          LoggingClusterState `json:"state,omitempty"`
	IndexUserState IndexUserState      `json:"indexUserState,omitempty"`
	ReadRole       string              `json:"readRole,omitempty"`
}

// +kubebuilder:object:root=true
type LoggingClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoggingCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoggingCluster{}, &LoggingClusterList{})
}

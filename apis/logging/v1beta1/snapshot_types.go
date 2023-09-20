package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SnapshotState string

const (
	SnapshotStatePending       SnapshotState = "Pending"
	SnapshotStateInProgress    SnapshotState = "In Progress"
	SnapshotStateCreated       SnapshotState = "Created"
	SnapshotStateCreateError   SnapshotState = "CreateError"
	SnapshotStateFetchError    SnapshotState = "FetchError"
	SnapshotStateFailedPartial SnapshotState = "CreatedWithErrors"
	SnapshotStateFailed        SnapshotState = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SnapshotSpec   `json:"spec,omitempty"`
	Status            SnapshotStatus `json:"status,omitempty"`
}

type SnapshotSpec struct {
	Indices            []string                    `json:"indices,omitempty"`
	IgnoreUnavailable  bool                        `json:"ignoreUnavailable,omitempty"`
	AllowPartial       bool                        `json:"allowPartial,omitempty"`
	IncludeGlobalState *bool                       `json:"includeGlobalState,omitempty"`
	Repository         corev1.LocalObjectReference `json:"repository"`
}

type SnapshotStatus struct {
	State           SnapshotState `json:"state,omitempty"`
	FailureMessage  string        `json:"failureMessage,omitempty"`
	SnapshotAPIName string        `json:"snapshotAPIName,omitempty"`
}

// +kubebuilder:object:root=true

// OpensearchRepositoryList contains a list of OpensearchRepository
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Snapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Snapshot{}, &SnapshotList{})
}

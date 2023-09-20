package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RecurringSnapshotState string

const (
	RecurringSnapshotStateCreated RecurringSnapshotState = "Created"
	RecurringSnapshotStateError   RecurringSnapshotState = "Error"
)

type RecurringSnapshotExecutionState string

const (
	RecurringSnapshotExecutionStateInProgress RecurringSnapshotExecutionState = "In Progress"
	RecurringSnapshotExecutionStateSuccess    RecurringSnapshotExecutionState = "Success"
	RecurringSnapshotExecutionStateRetrying   RecurringSnapshotExecutionState = "Retrying"
	RecurringSnapshotExecutionStateFailed     RecurringSnapshotExecutionState = "Failed"
	RecurringSnapshotExecutionStateTimedOut   RecurringSnapshotExecutionState = "Timed Out"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type RecurringSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RecurringSnapshotSpec   `json:"spec,omitempty"`
	Status            RecurringSnapshotStatus `json:"status,omitempty"`
}

type RecurringSnapshotSpec struct {
	Snapshot  SnapshotSpec                `json:"snapshot"`
	Creation  RecurringSnapshotCreation   `json:"creation"`
	Retention *RecurringSnapshotRetention `json:"retention,omitempty"`
}

type RecurringSnapshotCreation struct {
	CronSchedule string `json:"cronSchedule"`
	// TimeLimit is a duration string
	TimeLimit string `json:"timeLimit,omitempty"`
}

type RecurringSnapshotRetention struct {
	MaxAge   string `json:"maxAge,omitempty"`
	MaxCount *int32 `json:"maxCount,omitempty"`
}

type RecurringSnapshotStatus struct {
	State           RecurringSnapshotState            `json:"state,omitempty"`
	ExecutionStatus *RecurringSnapshotExecutionStatus `json:"executionStatus,omitempty"`
}

type RecurringSnapshotExecutionStatus struct {
	LastExecution metav1.Time                     `json:"lastExecution,omitempty"`
	Status        RecurringSnapshotExecutionState `json:"status,omitempty"`
	Message       string                          `json:"message,omitempty"`
	Cause         string                          `json:"cause,omitempty"`
}

// +kubebuilder:object:root=true

// OpensearchRepositoryList contains a list of OpensearchRepository
type RecurringSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RecurringSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RecurringSnapshot{}, &RecurringSnapshotList{})
}

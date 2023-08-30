package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OpensearchRepositoryState string

const (
	OpensearchRepositoryError   OpensearchRepositoryState = "Error"
	OpensearchRepositoryCreated OpensearchRepositoryState = "Created"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

type OpensearchRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OpensearchRepositorySpec   `json:"spec,omitempty"`
	Status            OpensearchRepositoryStatus `json:"status,omitempty"`
}

type OpensearchRepositorySpec struct {
	Settings             RepositorySettings             `json:"settings"`
	OpensearchClusterRef *opnimeta.OpensearchClusterRef `json:"opensearchClusterRef"`
}

type RepositorySettings struct {
	S3         *S3PathSettings     `json:"s3,omitempty"`
	FileSystem *FileSystemSettings `json:"filesystem,omitempty"`
}

type S3PathSettings struct {
	Bucket string `json:"bucket"`
	Folder string `json:"folder"`
}

type FileSystemSettings struct {
	Location string `json:"location"`
}

type OpensearchRepositoryStatus struct {
	State          OpensearchRepositoryState `json:"state,omitempty"`
	FailureMessage string                    `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true

// OpensearchRepositoryList contains a list of OpensearchRepository
type OpensearchRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpensearchRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpensearchRepository{}, &OpensearchRepositoryList{})
}

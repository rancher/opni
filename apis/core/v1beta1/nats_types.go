package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=password;nkey
type NatsAuthMethod string

const (
	NatsAuthPassword NatsAuthMethod = "password"
	NatsAuthNkey     NatsAuthMethod = "nkey"
)

type NatsClusterState string

const (
	NatsClusterStateError   NatsClusterState = "Error"
	NatsClusterStateWorking NatsClusterState = "Working"
	NatsClusterStateReady   NatsClusterState = "Ready"
)

type PVCSource struct {
	StorageClassName string                              `json:"storageClass,omitempty"`
	AccessModes      []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

type JetStreamPersistenceSpec struct {
	PVC      *PVCSource                   `json:"pvc,omitempty"`
	EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`
}

type FileStorageSpec struct {
	JetStreamPersistenceSpec `json:",inline"`
	Enabled                  *bool             `json:"enabled,omitempty"`
	Size                     resource.Quantity `json:"size,omitempty"`
}

type JetStreamSpec struct {
	Enabled           *bool             `json:"enabled,omitempty"`
	MemoryStorageSize resource.Quantity `json:"memoryStorageSize,omitempty"`
	FileStorage       FileStorageSpec   `json:"fileStorage,omitempty"`
}

type NatsSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=password
	AuthMethod NatsAuthMethod `json:"authMethod,omitempty"`
	// Username to use for authentication, if username auth is specified in
	// AuthMethod. If empty, defaults to "nats-user". If AuthMethod is "nkey",
	// this field is ignored.
	Username string `json:"username,omitempty"`
	// Number of nats server replicas. If not set, defaults to 3.
	Replicas *int32 `json:"replicas,omitempty"`
	// A secret containing a "password" item.	This secret must already exist
	// if specified.
	PasswordFrom *corev1.SecretKeySelector `json:"passwordFrom,omitempty"`
	NodeSelector map[string]string         `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration       `json:"tolerations,omitempty"`
	JetStream    JetStreamSpec             `json:"jetStream,omitempty"`
}

type NatsStatus struct {
	State            NatsClusterState          `json:"state,omitempty"`
	Replicas         int32                     `json:"replicas,omitempty"`
	AuthSecretKeyRef *corev1.SecretKeySelector `json:"authSecretKeyRef,omitempty"`
	NKeyUser         string                    `json:"nkeyUser,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NatsCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NatsSpec   `json:"spec,omitempty"`
	Status            NatsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NatsClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&NatsCluster{}, &NatsClusterList{},
	)
}

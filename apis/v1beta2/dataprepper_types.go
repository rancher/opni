package v1beta2

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DataPrepperState string

const (
	DataPrepperStatePending DataPrepperState = "pending"
	DataPrepperStateReady   DataPrepperState = "ready"
	DataprepperStateError   DataPrepperState = "error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DataPrepper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DataPrepperSpec   `json:"spec,omitempty"`
	Status            DataPrepperStatus `json:"status,omitempty"`
}

type DataPrepperStatus struct {
	Conditions []string         `json:"conditions,omitempty"`
	State      DataPrepperState `json:"state,omitempty"`
}

type DataPrepperSpec struct {
	*opnimeta.ImageSpec `json:",inline,omitempty"`
	// +kubebuilder:default:=latest
	Version string `json:"version"`
	// +optional
	DefaultRepo   *string                   `json:"defaultRepo,omitempty"`
	Opensearch    *OpensearchSpec           `json:"opensearch,omitempty"`
	Username      string                    `json:"username"`
	PasswordFrom  *corev1.SecretKeySelector `json:"passwordFrom,omitempty"`
	ClusterID     string                    `json:"cluster,omitempty"`
	NodeSelector  map[string]string         `json:"nodeSelector,omitempty"`
	Tolerations   []corev1.Toleration       `json:"tolerations,omitempty"`
	EnableTracing bool                      `json:"enableTracing,omitempty"`
}

type OpensearchSpec struct {
	Endpoint                 string `json:"endpoint,omitempty"`
	InsecureDisableSSLVerify bool   `json:"insecureDisableSSLVerify,omitempty"`
}

// +kubebuilder:object:root=true
type DataPrepperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataPrepper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPrepper{}, &DataPrepperList{})
}

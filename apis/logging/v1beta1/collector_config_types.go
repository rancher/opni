package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CollectorConfigSpec defines the desired state of CollectorConfig
type CollectorConfigSpec struct {
	// +kubebuilder:validation:Enum:=aks;eks;gke;k3s;rke;rke2;generic
	// +kubebuilder:validation:Required
	Provider LogProvider `json:"provider"`

	SELinuxEnabled bool     `json:"seLinuxEnabled,omitempty"`
	Selector       Selector `json:"selector,omitempty"`

	AKS  *AKSSpec  `json:"aks,omitempty"`
	EKS  *EKSSpec  `json:"eks,omitempty"`
	GKE  *GKESpec  `json:"gke,omitempty"`
	K3S  *K3SSpec  `json:"k3s,omitempty"`
	RKE  *RKESpec  `json:"rke,omitempty"`
	RKE2 *RKE2Spec `json:"rke2,omitempty"`
}

type SelectorConfig struct {
	Namespace string   `json:"namespace,omitempty"`
	PodNames  []string `json:"podNames,omitempty"`
}

type Selector struct {
	Include []SelectorConfig `json:"include,omitempty"`
	Exclude []SelectorConfig `json:"exclude,omitempty"`
}

// CollectorConfigStatus defines the observed state of CollectorConfig
type CollectorConfigStatus struct {
	Phase      string   `json:"phase,omitempty"`
	Message    string   `json:"message,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion

// CollectorConfig is the Schema for the logadapters API
type CollectorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CollectorConfigSpec   `json:"spec,omitempty"`
	Status CollectorConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CollectorConfigList contains a list of CollectorConfig
type CollectorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CollectorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CollectorConfig{}, &CollectorConfigList{})
}

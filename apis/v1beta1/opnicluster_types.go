/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpniClusterSpec defines the desired state of OpniCluster
type OpniClusterSpec struct {
	Services  ServicesSpec  `json:"services,omitempty"`
	Dashboard DashboardSpec `json:"dashboard,omitempty"`
	Elastic   ElasticSpec   `json:"elastic,omitempty"`
	Storage   StorageSpec   `json:"storage,omitempty"`
}

// OpniClusterStatus defines the observed state of OpniCluster
type OpniClusterStatus struct {
	Conditions []string `json:"conditions,omitempty"`
	State      string   `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`

// OpniCluster is the Schema for the opniclusters API
type OpniCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpniClusterSpec   `json:"spec,omitempty"`
	Status OpniClusterStatus `json:"status,omitempty"`
}

type ServicesSpec struct {
	Drain           DrainServiceSpec           `json:"drain,omitempty"`
	Inference       InferenceServiceSpec       `json:"inference,omitempty"`
	Preprocessing   PreprocessingServiceSpec   `json:"preprocessing,omitempty"`
	PayloadReceiver PayloadReceiverServiceSpec `json:"payloadReceiver,omitempty"`
}

type DrainServiceSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
}

type InferenceServiceSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
	// +optional
	PretrainedModels []PretrainedModelReference `json:"pretrainedModels,omitempty"`
}

type PretrainedModelReference struct {
	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

type PreprocessingServiceSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
}

type PayloadReceiverServiceSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
}

type TrainingControllerSpec struct {
	// +optional
	Image string `json:"image,omitempty"`
}

type DashboardSpec struct {
	// +optional
	Kibana KibanaSpec `json:"kibana,omitempty"`
	// +optional
	Grafana GrafanaSpec `json:"grafana,omitempty"`
}

type KibanaSpec struct {
	// +required
	Endpoint string `json:"endpoint"`
}

type GrafanaSpec struct {
	// TODO
}

type ElasticSpec struct {
	// +required
	Endpoint string `json:"endpoint"`
	// +required
	Credentials CredentialsSpec `json:"credentials"`
}

type StorageSpec struct {
	StorageClass string `json:"storageClass,omitempty"`
	S3           S3Spec `json:"s3,omitempty"`
}

type S3Spec struct {
	Endpoint    string          `json:"endpoint,omitempty"`
	Credentials CredentialsSpec `json:"credentials,omitempty"`
}

type CredentialsSpec struct {
	// +optional
	Keys *KeysSpec `json:"keys,omitempty"`

	// +optional
	SecretRef *corev1.SecretReference `json:"fromSecret,omitempty"`
}

type KeysSpec struct {
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

// +kubebuilder:object:root=true

// OpniClusterList contains a list of OpniCluster
type OpniClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpniCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpniCluster{}, &OpniClusterList{})
}

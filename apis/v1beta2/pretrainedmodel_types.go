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

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PretrainedModelSpec defines the desired state of PretrainedModel
type PretrainedModelSpec struct {
	// +kubebuilder:validation:Required
	ModelSource `json:"source"`
	// +optional
	Hyperparameters map[string]intstr.IntOrString `json:"hyperparameters,omitempty"`
	Replicas        *int32                        `json:"replicas,omitempty"`
}

type ModelSource struct {
	// +optional
	HTTP *HTTPSource `json:"http,omitempty"`
	// +optional
	Container *ContainerSource `json:"container,omitempty"`
}

type HTTPSource struct {
	// +kubebuilder:validation:Required
	URL string `json:"url"`
}

type ContainerSource struct {
	// +kubebuilder:validation:Required
	Image string `json:"image"`
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// PretrainedModelStatus defines the observed state of PretrainedModel
type PretrainedModelStatus struct {
	ConfigMap corev1.LocalObjectReference `json:"configMap,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:storageversion

// PretrainedModel is the Schema for the pretrainedmodels API
type PretrainedModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PretrainedModelSpec   `json:"spec,omitempty"`
	Status PretrainedModelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PretrainedModelList contains a list of PretrainedModel
type PretrainedModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PretrainedModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PretrainedModel{}, &PretrainedModelList{})
}

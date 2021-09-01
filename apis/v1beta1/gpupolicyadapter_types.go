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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ContainerRuntime string

const (
	// Auto will detect the container runtime based on the flavor of Kubernetes
	// in use. Containerd is the default, unless the cluster is using RKE.
	Auto       ContainerRuntime = "auto"
	Docker     ContainerRuntime = "docker"
	Containerd ContainerRuntime = "containerd"
)

// GpuPolicyAdapterSpec defines the desired state of GpuPolicyAdapter
type GpuPolicyAdapterSpec struct {
	// Version is the version of the GPU Operator. This is used for validator and
	// node status exporter components.
	Version          string           `json:"version"`
	ContainerRuntime ContainerRuntime `json:"containerRuntime"`
	// +optional
	DefaultRepo *string `json:"defaultRepo,omitempty"`
	RDMA        bool    `json:"rdma"`
}

// GpuPolicyAdapterStatus defines the observed state of GpuPolicyAdapter
type GpuPolicyAdapterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GpuPolicyAdapter is the Schema for the gpupolicyadapters API
type GpuPolicyAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GpuPolicyAdapterSpec   `json:"spec,omitempty"`
	Status GpuPolicyAdapterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GpuPolicyAdapterList contains a list of GpuPolicyAdapter
type GpuPolicyAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GpuPolicyAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GpuPolicyAdapter{}, &GpuPolicyAdapterList{})
}

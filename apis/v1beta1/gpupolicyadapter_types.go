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

//+kubebuilder:validation:Optional
package v1beta1

import (
	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ContainerRuntime string

const (
	// Auto will detect the container runtime based on the Kubernetes provider
	// in use. Containerd is the default, unless the cluster is using RKE.
	Auto       ContainerRuntime = "auto"
	Docker     ContainerRuntime = "docker"
	Containerd ContainerRuntime = "containerd"
	Crio       ContainerRuntime = "crio"
)

// GpuPolicyAdapterSpec defines the desired state of GpuPolicyAdapter
type GpuPolicyAdapterSpec struct {
	// +kubebuilder:validation:Enum={"auto","docker","containerd", "crio"}
	// +kubebuilder:default=auto
	ContainerRuntime ContainerRuntime `json:"containerRuntime,omitempty"`
	// +kubebuilder:validation:Enum={"auto","k3s","rke2","rke","none"}
	// +kubebuilder:default=auto
	KubernetesProvider string     `json:"kubernetesProvider,omitempty"`
	Images             ImagesSpec `json:"images,omitempty"`
	VGPU               *VGPUSpec  `json:"vgpu,omitempty"`
	// +kubebuilder:validation:Optional
	Template nvidiav1.ClusterPolicySpec `json:"template,omitempty"`
}

type VGPUSpec struct {
	LicenseConfigMap string `json:"licenseConfigMap,omitempty"`
	// +kubebuilder:validation:Enum={"nls","legacy"}
	LicenseServerKind string `json:"licenseServerKind,omitempty"`
}

type ImagesSpec struct {
	Driver        string `json:"driver,omitempty"`
	DriverManager string `json:"driverManager,omitempty"`
	DCGM          string `json:"dcgm,omitempty"`
	DCGMExporter  string `json:"dcgmExporter,omitempty"`
	DevicePlugin  string `json:"devicePlugin,omitempty"`
	GFD           string `json:"gfd,omitempty"`
	InitContainer string `json:"initContainer,omitempty"`
	Toolkit       string `json:"toolkit,omitempty"`
	Validator     string `json:"validator,omitempty"`
	MIGManager    string `json:"migManager,omitempty"`
}

// GpuPolicyAdapterStatus defines the observed state of GpuPolicyAdapter
type GpuPolicyAdapterStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
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

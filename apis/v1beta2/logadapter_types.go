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

// +kubebuilder:validation:Optional
package v1beta2

import (
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LogProvider string

const (
	LogProviderAKS     LogProvider = "aks"
	LogProviderEKS     LogProvider = "eks"
	LogProviderGKE     LogProvider = "gke"
	LogProviderK3S     LogProvider = "k3s"
	LogProviderRKE     LogProvider = "rke"
	LogProviderRKE2    LogProvider = "rke2"
	LogProviderGeneric LogProvider = "generic"
)

type ContainerEngine string

const (
	ContainerEngineSystemd ContainerEngine = "systemd"
	ContainerEngineOpenRC  ContainerEngine = "openrc"
)

// LogAdapterSpec defines the desired state of LogAdapter
type LogAdapterSpec struct {
	// +kubebuilder:validation:Enum:=aks;eks;gke;k3s;rke;rke2;generic
	// +kubebuilder:validation:Required
	Provider LogProvider `json:"provider"`

	OpniCluster      *OpniClusterNameSpec `json:"opniCluster,omitempty"`
	ControlNamespace *string              `json:"controlNamespace,omitempty"`

	ContainerLogDir string `json:"containerLogDir,omitempty"`
	SELinuxEnabled  bool   `json:"seLinuxEnabled,omitempty"`

	AKS  *AKSSpec  `json:"aks,omitempty"`
	EKS  *EKSSpec  `json:"eks,omitempty"`
	GKE  *GKESpec  `json:"gke,omitempty"`
	K3S  *K3SSpec  `json:"k3s,omitempty"`
	RKE  *RKESpec  `json:"rke,omitempty"`
	RKE2 *RKE2Spec `json:"rke2,omitempty"`

	FluentConfig     *FluentConfigSpec `json:"fluentConfig,omitempty"`
	RootFluentConfig *FluentConfigSpec `json:"rootFluentConfig,omitempty"`
}

type FluentConfigSpec struct {
	Fluentbit *loggingv1beta1.FluentbitSpec `json:"fluentbit,omitempty"`
	Fluentd   *loggingv1beta1.FluentdSpec   `json:"fluentd,omitempty"`
}

type OpniClusterNameSpec struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// Provider-specific settings are below.

type AKSSpec struct {
}

type EKSSpec struct {
}

type GKESpec struct {
}

type K3SSpec struct {
	ContainerEngine ContainerEngine `json:"containerEngine,omitempty"`
	LogPath         string          `json:"logPath,omitempty"`
}

type RKESpec struct {
	LogLevel opnimeta.LogLevel `json:"logLevel,omitempty"`
}

type RKE2Spec struct {
	LogPath string `json:"logPath,omitempty"`
}

// LogAdapterStatus defines the observed state of LogAdapter
type LogAdapterStatus struct {
	Phase      string   `json:"phase,omitempty"`
	Message    string   `json:"message,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion

// LogAdapter is the Schema for the logadapters API
type LogAdapter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogAdapterSpec   `json:"spec,omitempty"`
	Status LogAdapterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LogAdapterList contains a list of LogAdapter
type LogAdapterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LogAdapter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LogAdapter{}, &LogAdapterList{})
}

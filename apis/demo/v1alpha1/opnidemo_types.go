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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// OpniDemoSpec defines the desired state of OpniDemo
type OpniDemoSpec struct {
	Components             ComponentsSpec `json:"components"`
	MinioAccessKey         string         `json:"minioAccessKey"`
	MinioSecretKey         string         `json:"minioSecretKey"`
	MinioVersion           string         `json:"minioVersion"`
	NatsVersion            string         `json:"natsVersion"`
	NatsPassword           string         `json:"natsPassword"`
	NatsReplicas           int            `json:"natsReplicas"`
	NatsMaxPayload         int            `json:"natsMaxPayload"`
	NvidiaVersion          string         `json:"nvidiaVersion"`
	ElasticsearchUser      string         `json:"elasticsearchUser"`
	ElasticsearchPassword  string         `json:"elasticsearchPassword"`
	NulogServiceCPURequest string         `json:"nulogServiceCpuRequest"`
	NulogTrainImage        string         `json:"nulogTrainImage"`
	CreateKibanaDashboard  *bool          `json:"createKibanaDashboard,omitempty"`
	LoggingCRDNamespace    *string        `json:"loggingCrdNamespace,omitempty"`
}

type ComponentsSpec struct {
	Infra InfraStack `json:"infra"`
	Opni  OpniStack  `json:"opni"`
}

type InfraStack struct {
	DeployHelmController bool `json:"deployHelmController,omitempty"`
	DeployNvidiaPlugin   bool `json:"deployNvidiaPlugin,omitempty"`
}

type ChartOptions struct {
	Enabled bool `json:"enabled"`
	// +optional
	Set map[string]intstr.IntOrString `json:"set,omitempty"`
}

type OpniStack struct {
	DeployGpuServices bool `json:"deployGpuServices,omitempty"`

	// +optional
	RancherLogging ChartOptions `json:"rancherLogging,omitempty"`
	// +optional
	Minio ChartOptions `json:"minio,omitempty"`
	// +optional
	Nats ChartOptions `json:"nats,omitempty"`
	// +optional
	Elastic ChartOptions `json:"elastic,omitempty"`
}

// OpniDemoStatus defines the observed state of OpniDemo
type OpniDemoStatus struct {
	Conditions []string `json:"conditions,omitempty"`
	State      string   `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// OpniDemo is the Schema for the opnidemoes API
type OpniDemo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpniDemoSpec   `json:"spec,omitempty"`
	Status OpniDemoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpniDemoList contains a list of OpniDemo
type OpniDemoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpniDemo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpniDemo{}, &OpniDemoList{})
}

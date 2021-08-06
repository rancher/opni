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

type ImageSpec struct {
	Image            *string                       `json:"image,omitempty"`
	ImagePullPolicy  *corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// +kubebuilder:validation:Enum=username;nkey
type NatsAuthMethod string

const (
	NatsAuthUsername NatsAuthMethod = "username"
	NatsAuthNkey     NatsAuthMethod = "nkey"
)

type OpniClusterState string

const (
	OpniClusterStateError   OpniClusterState = "Error"
	OpniClusterStateWorking OpniClusterState = "Working"
	OpniClusterStateReady   OpniClusterState = "Ready"
)

// OpniClusterSpec defines the desired state of OpniCluster
type OpniClusterSpec struct {
	// +kubebuilder:default:=latest
	Version string `json:"version"`
	// +optional
	DefaultRepo *string `json:"defaultRepo,omitempty"`

	Services ServicesSpec `json:"services,omitempty"`
	Elastic  ElasticSpec  `json:"elastic,omitempty"`
	Nats     NatsSpec     `json:"nats,omitempty"`
	S3       S3Spec       `json:"s3,omitempty"`
}

// OpniClusterStatus defines the observed state of OpniCluster
type OpniClusterStatus struct {
	Conditions   []string         `json:"conditions,omitempty"`
	State        OpniClusterState `json:"state,omitempty"`
	NatsReplicas int32            `json:"natsReplicas,omitempty"`
	Auth         AuthStatus       `json:"auth,omitempty"`
}

type AuthStatus struct {
	NKeyUser         string                    `json:"nKeyUser,omitempty"`
	AuthSecretKeyRef *corev1.SecretKeySelector `json:"authSecretKeyRef,omitempty"`
	S3Endpoint       string                    `json:"s3Endpoint,omitempty"`
	S3AccessKey      *corev1.SecretKeySelector `json:"s3AccessKey,omitempty"`
	S3SecretKey      *corev1.SecretKeySelector `json:"s3SecretKey,omitempty"`
}

//+kubebuilder:webhook:path=/highlander-opni-io-v1beta1-opnicluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=opniclusters,verbs=create;update,versions=v1beta1,name=highlander.opni.io,admissionReviewVersions={v1,v1beta1}

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
	GPUController   GPUControllerServiceSpec   `json:"gpuController,omitempty"`
}

type DrainServiceSpec struct {
	Enabled   *bool `json:"enabled,omitempty"`
	ImageSpec `json:",inline,omitempty"`
}

type InferenceServiceSpec struct {
	Enabled   *bool `json:"enabled,omitempty"`
	ImageSpec `json:",inline,omitempty"`
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
	Enabled   *bool `json:"enabled,omitempty"`
	ImageSpec `json:",inline,omitempty"`
}

type PayloadReceiverServiceSpec struct {
	Enabled   *bool `json:"enabled,omitempty"`
	ImageSpec `json:",inline,omitempty"`
}

type GPUControllerServiceSpec struct {
	Enabled   *bool `json:"enabled,omitempty"`
	ImageSpec `json:",inline,omitempty"`
}

type ElasticSpec struct {
	// +kubebuilder:default:=latest
	Version      string                       `json:"version"`
	Workloads    ElasticWorkloadSpec          `json:"workloads,omitempty"`
	DefaultRepo  *string                      `json:"defaultRepo,omitempty"`
	Image        *ImageSpec                   `json:"image,omitempty"`
	KibanaImage  *ImageSpec                   `json:"kibanaImage,omitempty"`
	Persistence  *PersistenceSpec             `json:"persistence,omitempty"`
	ConfigSecret *corev1.LocalObjectReference `json:"configSecret,omitempty"`
}

type ElasticWorkloadSpec struct {
	Master ElasticWorkloadMasterSpec `json:"master,omitempty"`
	Data   ElasticWorkloadDataSpec   `json:"data,omitempty"`
	Client ElasticWorkloadClientSpec `json:"client,omitempty"`
	Kibana ElasticWorkloadKibanaSpec `json:"kibana,omitempty"`
}

type ElasticWorkloadMasterSpec struct {
	Replicas  *int32                       `json:"replicas,omitempty"`
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity  *corev1.Affinity             `json:"affinity,omitempty"`
}

type ElasticWorkloadDataSpec struct {
	// +kubebuilder:default:=true
	DedicatedPod bool                         `json:"dedicatedPod,omitempty"`
	Replicas     *int32                       `json:"replicas,omitempty"`
	Resources    *corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity     *corev1.Affinity             `json:"affinity,omitempty"`
}

type ElasticWorkloadClientSpec struct {
	// +kubebuilder:default:=true
	DedicatedPod bool                         `json:"dedicatedPod,omitempty"`
	Replicas     *int32                       `json:"replicas,omitempty"`
	Resources    *corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity     *corev1.Affinity             `json:"affinity,omitempty"`
}

type ElasticWorkloadKibanaSpec struct {
	Replicas  *int32                       `json:"replicas,omitempty"`
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity  *corev1.Affinity             `json:"affinity,omitempty"`
}

type PersistenceSpec struct {
	Enabled          bool                                `json:"enabled,omitempty"`
	StorageClassName *string                             `json:"storageClass,omitempty"`
	AccessModes      []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	Request          string                              `json:"request,omitempty"`
}

type S3Spec struct {
	// If set, Opni will deploy an S3 pod to use internally.
	// Cannot be set at the same time as `external`.
	Internal *InternalSpec `json:"internal,omitempty"`
	// If set, Opni will connect to an external S3 endpoint.
	// Cannot be set at the same time as `internal`.
	External *ExternalSpec `json:"external,omitempty"`
}

type InternalSpec struct {
}

type ExternalSpec struct {
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:Required
	// Reference to a secret containing "accessKey" and "secretKey" items. This
	// secret must already exist.
	Credentials *corev1.SecretReference `json:"credentials,omitempty"`
}

type NatsSpec struct {
	// +required
	// +kubebuilder:default:=username
	AuthMethod   NatsAuthMethod            `json:"authMethod,omitempty"`
	Username     string                    `json:"username,omitempty"`
	Replicas     *int32                    `json:"replicas,omitempty"`
	PasswordFrom *corev1.SecretKeySelector `json:"passwordFrom,omitempty"`
	NatsURL      string                    `json:"natsURL"`
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

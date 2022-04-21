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
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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

	Services             ServicesSpec                  `json:"services,omitempty"`
	Opensearch           OpensearchClusterSpec         `json:"opensearch,omitempty"`
	Nats                 NatsSpec                      `json:"nats,omitempty"`
	S3                   S3Spec                        `json:"s3,omitempty"`
	NulogHyperparameters map[string]intstr.IntOrString `json:"nulogHyperparameters,omitempty"`
	DeployLogCollector   *bool                         `json:"deployLogCollector"`
	GlobalNodeSelector   map[string]string             `json:"globalNodeSelector,omitempty"`
	GlobalTolerations    []corev1.Toleration           `json:"globalTolerations,omitempty"`
}

// OpniClusterStatus defines the observed state of OpniCluster
type OpniClusterStatus struct {
	Conditions              []string         `json:"conditions,omitempty"`
	State                   OpniClusterState `json:"state,omitempty"`
	OpensearchState         OpensearchStatus `json:"opensearchState,omitempty"`
	LogCollectorState       OpniClusterState `json:"logState,omitempty"`
	NatsReplicas            int32            `json:"natsReplicas,omitempty"`
	Auth                    AuthStatus       `json:"auth,omitempty"`
	PrometheusRuleNamespace string           `json:"prometheusRuleNamespace,omitempty"`
}

type AuthStatus struct {
	NKeyUser                   string                    `json:"nKeyUser,omitempty"`
	NatsAuthSecretKeyRef       *corev1.SecretKeySelector `json:"natsAuthSecretKeyRef,omitempty"`
	GenerateOpensearchHash     *bool                     `json:"generateOpensearchHash"`
	OpensearchAuthSecretKeyRef *corev1.SecretKeySelector `json:"opensearchAuthSecretKeyRef,omitempty"`
	S3Endpoint                 string                    `json:"s3Endpoint,omitempty"`
	S3AccessKey                *corev1.SecretKeySelector `json:"s3AccessKey,omitempty"`
	S3SecretKey                *corev1.SecretKeySelector `json:"s3SecretKey,omitempty"`
}

type OpensearchStatus struct {
	IndexState  OpniClusterState `json:"indexState,omitempty"`
	Version     *string          `json:"version,omitempty"`
	Initialized bool             `json:"initialized,omitempty"`
}

//+kubebuilder:webhook:path=/highlander-opni-io-v1beta1-opnicluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=opniclusters,verbs=create;update,versions={v1beta1,v1beta2},name=highlander.opni.io,admissionReviewVersions={v1,v1beta1}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="State",type=boolean,JSONPath=`.status.state`
// +kubebuilder:storageversion

// OpniCluster is the Schema for the opniclusters API
type OpniCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpniClusterSpec   `json:"spec,omitempty"`
	Status OpniClusterStatus `json:"status,omitempty"`
}

type ServicesSpec struct {
	Drain             DrainServiceSpec             `json:"drain,omitempty"`
	Inference         InferenceServiceSpec         `json:"inference,omitempty"`
	Preprocessing     PreprocessingServiceSpec     `json:"preprocessing,omitempty"`
	PayloadReceiver   PayloadReceiverServiceSpec   `json:"payloadReceiver,omitempty"`
	GPUController     GPUControllerServiceSpec     `json:"gpuController,omitempty"`
	Metrics           MetricsServiceSpec           `json:"metrics,omitempty"`
	OpensearchFetcher OpensearchFetcherServiceSpec `json:"opensearchFetcher,omitempty"`
}

type DrainServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
	Replicas           *int32              `json:"replicas,omitempty"`
}

type InferenceServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool                         `json:"enabled,omitempty"`
	PretrainedModels   []corev1.LocalObjectReference `json:"pretrainedModels,omitempty"`
	NodeSelector       map[string]string             `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration           `json:"tolerations,omitempty"`
}

type PreprocessingServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
	Replicas           *int32              `json:"replicas,omitempty"`
}

type PayloadReceiverServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
}

type GPUControllerServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	RuntimeClass       *string             `json:"runtimeClass,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
}

type MetricsServiceSpec struct {
	opnimeta.ImageSpec  `json:",inline,omitempty"`
	Enabled             *bool                         `json:"enabled,omitempty"`
	NodeSelector        map[string]string             `json:"nodeSelector,omitempty"`
	Tolerations         []corev1.Toleration           `json:"tolerations,omitempty"`
	PrometheusEndpoint  string                        `json:"prometheusEndpoint,omitempty"`
	PrometheusReference *opnimeta.PrometheusReference `json:"prometheus,omitempty"`
}

type InsightsServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
}

type UIServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
}

type OpensearchFetcherServiceSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	NodeSelector       map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations        []corev1.Toleration `json:"tolerations,omitempty"`
}

type OpensearchClusterSpec struct {
	ExternalOpensearch       *opnimeta.OpensearchClusterRef `json:"externalOpensearch"`
	Version                  string                         `json:"version"`
	Workloads                OpensearchWorkloadSpec         `json:"workloads,omitempty"`
	DefaultRepo              *string                        `json:"defaultRepo,omitempty"`
	Image                    *opnimeta.ImageSpec            `json:"image,omitempty"`
	DashboardsImage          *opnimeta.ImageSpec            `json:"dashboardsImage,omitempty"`
	Persistence              *opnimeta.PersistenceSpec      `json:"persistence,omitempty"`
	EnableLogIndexManagement *bool                          `json:"enableLogIndexManagement"`
	// Secret containing an item "logging.yml" with the contents of the
	// elasticsearch logging config.
	ConfigSecret *corev1.LocalObjectReference `json:"configSecret,omitempty"`
	// Reference to a secret containing the desired admin password
	AdminPasswordFrom *corev1.SecretKeySelector `json:"adminPasswordFrom,omitempty"`
}

type OpensearchWorkloadSpec struct {
	Master     OpensearchWorkloadOptions `json:"master,omitempty"`
	Data       OpensearchWorkloadOptions `json:"data,omitempty"`
	Client     OpensearchWorkloadOptions `json:"client,omitempty"`
	Dashboards OpensearchWorkloadOptions `json:"dashboards,omitempty"`
}

type OpensearchWorkloadOptions struct {
	Replicas     *int32                       `json:"replicas,omitempty"`
	Resources    *corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity     *corev1.Affinity             `json:"affinity,omitempty"`
	NodeSelector map[string]string            `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration          `json:"tolerations,omitempty"`
}

type S3Spec struct {
	// If set, Opni will deploy an S3 pod to use internally.
	// Cannot be set at the same time as `external`.
	Internal *InternalSpec `json:"internal,omitempty"`
	// If set, Opni will connect to an external S3 endpoint.
	// Cannot be set at the same time as `internal`.
	External *ExternalSpec `json:"external,omitempty"`
	// Bucket used to persist nulog models.  If not set will use
	// opni-nulog-models.
	NulogS3Bucket string `json:"nulogS3Bucket,omitempty"`
	// Bucket used to persiste drain models.  It not set will use
	// opni-drain-models
	DrainS3Bucket string `json:"drainS3Bucket,omitempty"`
}

type InternalSpec struct {
	// Persistence configuration for internal S3 deployment. If unset, internal
	// S3 storage is not persistent.
	Persistence *opnimeta.PersistenceSpec `json:"persistence,omitempty"`
}

type ExternalSpec struct {
	// +kubebuilder:validation:Required
	// External S3 endpoint URL.
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:Required
	// Reference to a secret containing "accessKey" and "secretKey" items. This
	// secret must already exist if specified.
	Credentials *corev1.SecretReference `json:"credentials,omitempty"`
}

type NatsSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=username
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

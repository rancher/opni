package v1beta1

import (
	"strconv"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v1beta2 spec
type AlertingSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:default=9093
	WebPort int `json:"webPort,omitempty"`
	//+kubebuilder:default=9094
	ClusterPort int `json:"clusterPort,omitempty"`
	//+kubebuilder:default="ClusterIP"
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	//+kubebuilder:default="500Mi"
	Storage string `json:"storage,omitempty"`
	//+kubebuilder:default="500m"
	CPU string `json:"cpu,omitempty"`
	//+kubebuilder:default="200Mi"
	Memory string `json:"memory,omitempty"`
	//+kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`
	//+kubebuilder:default="1m0s"
	ClusterSettleTimeout string `json:"clusterSettleTimeout,omitempty"`
	//+kubebuilder:default="1m0s"
	ClusterPushPullInterval string `json:"clusterPushPullInterval,omitempty"`
	//+kubebuilder:default="200ms"
	ClusterGossipInterval string `json:"clusterGossipInterval,omitempty"`
	ConfigName            string `json:"configName,omitempty"`
	//+kubebuilder:default="/var/lib"
	DataMountPath       string                      `json:"dataMountPath,omitempty"`
	GatewayVolumeMounts []opnimeta.ExtraVolumeMount `json:"alertVolumeMounts,omitempty"`
	//! deprecated
	RawAlertManagerConfig string `json:"rawConfigMap,omitempty"`
	//! deprecated
	RawInternalRouting string `json:"rawInternalRouting,omitempty"`
}

type CortexSpec struct {
	Enabled         *bool                              `json:"enabled,omitempty"`
	CortexWorkloads *cortexops.CortexWorkloadsConfig   `json:"cortexWorkloads,omitempty"`
	CortexConfig    *cortexops.CortexApplicationConfig `json:"cortexConfig,omitempty"`
}

type GrafanaSpec struct {
	*cortexops.GrafanaConfig `json:",inline,omitempty"`
	// Contains any additional configuration or overrides for the Grafana
	// installation spec.
	grafanav1alpha1.GrafanaSpec `json:",inline,omitempty"`
}

type MonitoringClusterSpec struct {
	Gateway corev1.LocalObjectReference `json:"gateway,omitempty"`
	Cortex  CortexSpec                  `json:"cortex,omitempty"`
	Grafana GrafanaSpec                 `json:"grafana,omitempty"`
}

type MonitoringClusterStatus struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Cortex          CortexStatus      `json:"cortex,omitempty"`
}

type CortexStatus struct {
	Version        string                    `json:"version,omitempty"`
	WorkloadsReady bool                      `json:"workloadsReady,omitempty"`
	Conditions     []string                  `json:"conditions,omitempty"`
	WorkloadStatus map[string]WorkloadStatus `json:"workloadStatus,omitempty"`
}

type WorkloadStatus struct {
	Ready   bool   `json:"ready,omitempty"`
	Message string `json:"conditions,omitempty"`
}

const (
	InternalRevisionAnnotation      string = "internal.opni.io/revision"
	InternalSchemalessAnnotation    string = "internal.opni.io/schemaless"
	MonitoringClusterTargetRevision int64  = 1
)

func MonitoringClusterTargetRevisionString() string {
	return strconv.FormatInt(MonitoringClusterTargetRevision, 10)
}

func GetMonitoringClusterRevision(t interface {
	GetAnnotations() map[string]string
}) int64 {
	annotations := t.GetAnnotations()
	if annotations == nil {
		return 0
	}
	revisionStr, ok := annotations[InternalRevisionAnnotation]
	if !ok {
		return 0
	}
	rev, err := strconv.ParseInt(revisionStr, 10, 64)
	if err != nil {
		return 0
	}
	return rev
}

func SetMonitoringClusterRevision(t interface {
	GetAnnotations() map[string]string
	SetAnnotations(map[string]string)
}, rev int64) {
	annotations := t.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[InternalRevisionAnnotation] = strconv.FormatInt(rev, 10)
	t.SetAnnotations(annotations)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:metadata:annotations=internal.opni.io/schemaless=true
type MonitoringCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	//+kubebuilder:validation:Schemaless
	//+kubebuilder:pruning:PreserveUnknownFields
	//+kubebuilder:validation:Type=object
	Spec MonitoringClusterSpec `json:"spec,omitempty"`

	//+kubebuilder:validation:Schemaless
	//+kubebuilder:pruning:PreserveUnknownFields
	//+kubebuilder:validation:Type=object
	Status MonitoringClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MonitoringClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitoringCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&MonitoringCluster{}, &MonitoringClusterList{},
	)
}

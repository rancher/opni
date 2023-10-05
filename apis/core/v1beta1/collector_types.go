package v1beta1

import (
	"time"

	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CollectorState string

const (
	CollectorStatePending CollectorState = "pending"
	CollectorStateReady   CollectorState = "ready"
	CollectorStateError   CollectorState = "error"
)

type CollectorSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
	AgentEndpoint      string                       `json:"agentEndpoint,omitempty"`
	SystemNamespace    string                       `json:"systemNamespace,omitempty"`
	LoggingConfig      *corev1.LocalObjectReference `json:"loggingConfig,omitempty"`
	MetricsConfig      *corev1.LocalObjectReference `json:"metricsConfig,omitempty"`
	ConfigReloader     *ConfigReloaderSpec          `json:"configReloader,omitempty"`
	LogLevel           string                       `json:"logLevel,omitempty"`
	OTELConfigSpec     *OTELConfigSpec              `json:"otelCollectorSpec,omitempty"`
}

type ConfigReloaderSpec struct {
	opnimeta.ImageSpec `json:",inline,omitempty"`
}

type OTELConfigSpec struct {
	Processors OTELProcessors `json:"processors,omitempty"`
	Exporters  OTELExporters  `json:"exporters,omitempty"`
}

type OTELProcessors struct {
	Batch         BatchProcessorConfig         `json:"batch,omitempty"`
	MemoryLimiter MemoryLimiterProcessorConfig `json:"memoryLimiter,omitempty"`
}

type OTELExporters struct {
	OTLPHTTP OTLPHTTPExporterConfig `json:"otlphttp,omitempty"`
}

type MemoryLimiterProcessorConfig struct {
	// CheckInterval is the time between measurements of memory usage for the
	// purposes of avoiding going over the limits. Defaults to zero, so no
	// checks will be performed.
	// +kubebuilder:default="1s"
	// +kubebuilder:validation:Format=duration
	CheckInterval time.Duration `json:"checkInterval,omitempty"`

	// MemoryLimitMiB is the maximum amount of memory, in MiB, targeted to be
	// allocated by the process.
	// +kubebuilder:default=1000
	MemoryLimitMiB uint32 `json:"limitMib,omitempty"`

	// MemorySpikeLimitMiB is the maximum, in MiB, spike expected between the
	// measurements of memory usage.
	// +kubebuilder:default=350
	MemorySpikeLimitMiB uint32 `json:"spikeLimitMib,omitempty"`

	// MemoryLimitPercentage is the maximum amount of memory, in %, targeted to be
	// allocated by the process. The fixed memory settings MemoryLimitMiB has a higher precedence.
	MemoryLimitPercentage uint32 `json:"limitPercentage,omitempty"`

	// MemorySpikePercentage is the maximum, in percents against the total memory,
	// spike expected between the measurements of memory usage.
	MemorySpikePercentage uint32 `json:"spikeLimitPercentage,omitempty"`
}

type BatchProcessorConfig struct {
	// Timeout sets the time after which a batch will be sent regardless of size.
	// When this is set to zero, batched data will be sent immediately.
	// +kubebuilder:default="15s"
	// +kubebuilder:validation:Format=duration
	Timeout time.Duration `json:"timeout,omitempty"`

	// SendBatchSize is the size of a batch which after hit, will trigger it to be sent.
	// When this is set to zero, the batch size is ignored and data will be sent immediately
	// subject to only send_batch_max_size.
	// +kubebuilder:default=1000
	SendBatchSize uint32 `json:"sendBatchSize,omitempty"`

	// SendBatchMaxSize is the maximum size of a batch. It must be larger than SendBatchSize.
	// Larger batches are split into smaller units.
	// Default value is 0, that means no maximum size.
	SendBatchMaxSize uint32 `json:"sendBatchMaxSize,omitempty"`

	// MetadataKeys is a list of client.Metadata keys that will be
	// used to form distinct batchers.  If this setting is empty,
	// a single batcher instance will be used.  When this setting
	// is not empty, one batcher will be used per distinct
	// combination of values for the listed metadata keys.
	//
	// Empty value and unset metadata are treated as distinct cases.
	//
	// Entries are case-insensitive.  Duplicated entries will
	// trigger a validation error.
	MetadataKeys []string `json:"metadataKeys,omitempty"`

	// MetadataCardinalityLimit indicates the maximum number of
	// batcher instances that will be created through a distinct
	// combination of MetadataKeys.
	MetadataCardinalityLimit uint32 `json:"metadataCardinalityLimit,omitempty"`
}

type OTLPHTTPSendingQueue struct {
	// Enabled indicates whether to not enqueue batches before sending to the consumerSender.
	Enabled bool `json:"enabled,omitempty"`
	// NumConsumers is the number of consumers from the queue.
	NumConsumers int `json:"numConsumers,omitempty"`
	// QueueSize is the maximum number of batches allowed in queue at a given time.
	QueueSize int `json:"queueSize,omitempty"`
}
type OTLPHTTPExporterConfig struct {
	SendingQueue OTLPHTTPSendingQueue `json:"sendingQueue,omitempty"`
}

// CollectorStatus defines the observed state of Collector
type CollectorStatus struct {
	Conditions []string       `json:"conditions,omitempty"`
	State      CollectorState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion

// Collector is the Schema for the logadapters API
type Collector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CollectorSpec   `json:"spec,omitempty"`
	Status CollectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CollectorList contains a list of Collector
type CollectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Collector `json:"items"`
}

func CollectorCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(Collector{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:          GroupVersion.WithKind("Collector"),
		PluralName:   "collectors",
		Status:       true,
		Schema:       schema,
		NonNamespace: true,
	}, nil
}

func init() {
	SchemeBuilder.Register(&Collector{}, &CollectorList{})
}

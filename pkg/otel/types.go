package otel

import (
	"bytes"
	"fmt"
	"time"

	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v2"
)

const (
	CollectorName             = "opni"
	MetricsCrdName            = "opni-monitoring"
	MetricsFeatureFlag        = "otel-metrics"
	MetricsServiceAccountName = "opni-otel-prometheus-agent"
)

func AgentEndpoint(serviceName string) string {
	return fmt.Sprintf("http://%s/api/agent/otel", serviceName)
}

type NodeConfig struct {
	Instance      string
	Logs          LoggingConfig
	Metrics       MetricsConfig
	Containerized bool
	LogLevel      string
}

type AggregatorConfig struct {
	AgentEndpoint string
	LogsEnabled   bool
	Metrics       MetricsConfig
	Containerized bool
	LogLevel      string
	OTELConfig    OTELConfigSpec
}

type LoggingConfig struct {
	Enabled   bool
	Receivers []string
}

type MetricsConfig struct {
	Enabled             bool
	LogLevel            string
	ListenPort          int
	RemoteWriteEndpoint string
	WALDir              string
	// It becomes easier to marshal []promcfg.ScrapeConfig to yaml strings
	DiscoveredScrapeCfg string
	Spec                *OTELSpec
}

type OTELSpec struct {
	AdditionalScrapeConfigs []*ScrapeConfig `json:"additionalScrapeConfigs,omitempty"`
	Wal                     *WALConfig      `json:"wal,omitempty"`
	HostMetrics             *bool           `json:"hostMetrics,omitempty"`
}

type OTELConfigSpec struct {
	// Batch Processor Configs
	BatchProcessor BatchConfig `json:"batch,omitempty"`

	// Memory Limiter Processor Configs
	MemoryLimiterProcessor MemoryLimiterConfig `json:"memoryLimiter,omitempty"`
}

// BatchConfig defines configuration for batch processor.
type BatchConfig struct {
	// Timeout sets the time after which a batch will be sent regardless of size.
	// When this is set to zero, batched data will be sent immediately.
	Timeout time.Duration `json:"timeout,omitempty"`

	// SendBatchSize is the size of a batch which after hit, will trigger it to be sent.
	// When this is set to zero, the batch size is ignored and data will be sent immediately
	// subject to only send_batch_max_size.
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

// MemoryLimiterConfig defines configuration for memory memoryLimiter processor.
type MemoryLimiterConfig struct {
	// CheckInterval is the time between measurements of memory usage for the
	// purposes of avoiding going over the limits. Defaults to zero, so no
	// checks will be performed.
	CheckInterval time.Duration `json:"checkInterval,omitempty"`

	// MemoryLimitMiB is the maximum amount of memory, in MiB, targeted to be
	// allocated by the process.
	MemoryLimitMiB uint32 `json:"limitMib,omitempty"`

	// MemorySpikeLimitMiB is the maximum, in MiB, spike expected between the
	// measurements of memory usage.
	MemorySpikeLimitMiB uint32 `json:"spikeLimitMib,omitempty"`

	// MemoryLimitPercentage is the maximum amount of memory, in %, targeted to be
	// allocated by the process. The fixed memory settings MemoryLimitMiB has a higher precedence.
	MemoryLimitPercentage uint32 `json:"limitPercentage,omitempty"`

	// MemorySpikePercentage is the maximum, in percents against the total memory,
	// spike expected between the measurements of memory usage.
	MemorySpikePercentage uint32 `json:"spikeLimitPercentage,omitempty"`
}

type ScrapeConfig struct {
	JobName        string   `json:"jobName,omitempty"`
	Targets        []string `json:"targets,omitempty"`
	ScrapeInterval string   `json:"scrapeInterval,omitempty"`
}

type WALConfig struct {
	Enabled           bool                 `json:"enabled,omitempty"`
	BufferSize        int64                `json:"bufferSize,omitempty"`
	TruncateFrequency *durationpb.Duration `json:"truncateFrequency,omitempty"`
}

func (in *MetricsConfig) DeepCopyInto(out *MetricsConfig) {
	*out = *in
	if in.Spec != nil {
		in, out := &in.Spec, &out.Spec
		*out = new(OTELSpec)
		(*in).DeepCopyInto(*out)
	}
}

func (in *MetricsConfig) DeepCopy() *MetricsConfig {
	if in == nil {
		return nil
	}
	out := new(MetricsConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *OTELSpec) DeepCopyInto(out *OTELSpec) {
	*out = *in
	if in.AdditionalScrapeConfigs != nil {
		in, out := &in.AdditionalScrapeConfigs, &out.AdditionalScrapeConfigs
		*out = make([]*ScrapeConfig, len(*in))
		for i := range *in {
			(*out)[i] = (*in)[i].DeepCopy()
		}
	}
	if in.Wal != nil {
		in, out := &in.Wal, &out.Wal
		*out = new(WALConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.HostMetrics != nil {
		in, out := &in.HostMetrics, &out.HostMetrics
		*out = new(bool)
		**out = **in
	}
}

func (in *OTELSpec) DeepCopy() *OTELSpec {
	if in == nil {
		return nil
	}
	out := new(OTELSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ScrapeConfig) DeepCopyInto(out *ScrapeConfig) {
	*out = *in
	if in.Targets != nil {
		in, out := &in.Targets, &out.Targets
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *ScrapeConfig) DeepCopy() *ScrapeConfig {
	if in == nil {
		return nil
	}
	out := new(ScrapeConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *WALConfig) DeepCopyInto(out *WALConfig) {
	*out = *in
	if in.TruncateFrequency != nil {
		out.TruncateFrequency = util.ProtoClone(in.TruncateFrequency)
	}
}

func (in *WALConfig) DeepCopy() *WALConfig {
	if in == nil {
		return nil
	}
	out := new(WALConfig)
	in.DeepCopyInto(out)
	return out
}

func (d NodeConfig) MetricReceivers() []string {
	res := []string{}
	if d.Metrics.Enabled {
		if lo.FromPtrOr(d.Metrics.Spec.HostMetrics, false) {
			res = append(res, "hostmetrics")
			if d.Containerized {
				res = append(res, "kubeletstats")
			}
		}
	}
	return res
}

func (o AggregatorConfig) MetricReceivers() []string {
	res := []string{}
	if o.Metrics.Enabled {
		if len(o.Metrics.Spec.AdditionalScrapeConfigs) > 0 {
			res = append(res, "prometheus/additional")
		}
		if len(o.Metrics.DiscoveredScrapeCfg) > 0 {
			res = append(res, "prometheus/discovered")
		}
	}
	return res
}

func PromCfgToString(cfgs []yaml.MapSlice) string {
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	err := yamlEncoder.Encode(&cfgs)
	if err != nil {
		return ""
	}
	return b.String()
}

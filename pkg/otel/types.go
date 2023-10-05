package otel

import (
	"bytes"
	"fmt"

	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
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
	OTELConfig    AggregatorOTELConfig
}

type AggregatorOTELConfig struct {
	Processors *AggregatorOTELProcessors
	// Exporters  *AggregatorOTELExporters
}

type AggregatorOTELProcessors struct {
	Batch         batchprocessor.Config
	MemoryLimiter memorylimiterprocessor.Config
}

// type AggregatorOTELExporters struct {
// 	OTLPHTTP otlphttpexporter.Config
// }

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

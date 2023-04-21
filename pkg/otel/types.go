package otel

import (
	"bytes"
	"fmt"

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
}

type AggregatorConfig struct {
	AgentEndpoint string
	LogsEnabled   bool
	Metrics       MetricsConfig
	Containerized bool
}

type LoggingConfig struct {
	Enabled   bool
	Receivers []string
}

type MetricsConfig struct {
	Enabled             bool
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

func (o *OTELSpec) DeepCopyInto(out *OTELSpec) {
	out.HostMetrics = o.HostMetrics
	out.AdditionalScrapeConfigs = o.AdditionalScrapeConfigs
	out.Wal = o.Wal
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

func (d NodeConfig) MetricReceivers() []string {
	res := []string{}
	if d.Metrics.Enabled {
		res = append(res, "prometheus/self")
		if d.Metrics.Spec.HostMetrics != nil && *d.Metrics.Spec.HostMetrics {
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
		res = append(res, "prometheus/self")
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

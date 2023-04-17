package otel

import (
	"bytes"
	"fmt"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"gopkg.in/yaml.v2"
)

const (
	CollectorName      = "opni"
	MetricsCrdName     = "opni-monitoring"
	MetricsFeatureFlag = "otel-metrics"
)

func AgentEndpoint(serviceName string) string {
	return fmt.Sprintf("http://%s/api/agent/otel", serviceName)
}

type NodeConfig struct {
	Instance      string
	ReceiverFile  string
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
	Spec                *node.OTELSpec
}

func (d NodeConfig) MetricReceivers() []string {
	res := []string{}
	if d.Metrics.Enabled {
		res = append(res, "prometheus/self")
		if d.Metrics.Spec.HostMetrics {
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

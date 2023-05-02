package node

import (
	"github.com/rancher/opni/pkg/otel"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func (m *MetricsCapabilitySpec) RuleDiscoveryEnabled() bool {
	return m.Rules != nil && m.Rules.Discovery != nil
}

func IsDefaultConfig(trailer metadata.MD) bool {
	if len(trailer["is-default-config"]) > 0 {
		return trailer["is-default-config"][0] == "true"
	}
	return false
}

func DefaultConfigMetadata() metadata.MD {
	return metadata.Pairs("is-default-config", "true")
}

func (in *OTELSpec) DeepCopyInto(out *OTELSpec) {
	proto.Merge(out, in)
}

func CompatOTELStruct(in *OTELSpec) *otel.OTELSpec {
	if in == nil {
		return nil
	}
	out := &otel.OTELSpec{
		AdditionalScrapeConfigs: make([]*otel.ScrapeConfig, len(in.AdditionalScrapeConfigs)),
		HostMetrics:             in.HostMetrics,
	}
	if in.Wal != nil {
		out.Wal = &otel.WALConfig{
			Enabled:           in.Wal.Enabled,
			BufferSize:        in.Wal.BufferSize,
			TruncateFrequency: in.Wal.TruncateFrequency,
		}
	}

	for _, s := range in.AdditionalScrapeConfigs {
		out.AdditionalScrapeConfigs = append(out.AdditionalScrapeConfigs, &otel.ScrapeConfig{
			JobName:        s.JobName,
			Targets:        s.Targets,
			ScrapeInterval: s.ScrapeInterval,
		})
	}
	return out
}

package node

import (
	"os"

	cli "github.com/rancher/opni/internal/codegen/cli"
	"github.com/rancher/opni/pkg/otel"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/dryrun"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/tui"
	"golang.org/x/term"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (m *MetricsCapabilityConfig) RuleDiscoveryEnabled() bool {
	return m.Rules != nil && m.Rules.Discovery != nil
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
			Enabled:           in.GetWal().GetEnabled(),
			BufferSize:        in.GetWal().GetBufferSize(),
			TruncateFrequency: in.GetWal().GetTruncateFrequency(),
		}
	}

	for _, s := range in.AdditionalScrapeConfigs {
		out.AdditionalScrapeConfigs = append(out.AdditionalScrapeConfigs, &otel.ScrapeConfig{
			JobName:        s.GetJobName(),
			Targets:        s.GetTargets(),
			ScrapeInterval: s.GetScrapeInterval(),
		})
	}
	return out
}

// Implements driverutil.ContextKeyable
func (g *GetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("node")
}

// Implements driverutil.ContextKeyable
func (g *SetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("node")
}

// Implements driverutil.ContextKeyable
func (g *ResetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("node")
}

// Implements driverutil.ContextKeyable
func (g *DryRunRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("node")
}

// Implements driverutil.ContextKeyable
func (g *ConfigurationHistoryRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("node")
}

func (h *ConfigurationHistoryResponse) RenderText(out cli.Writer) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		out.Println(driverutil.MarshalConfigJson(h))
		return
	}
	ui := tui.NewHistoryUI(h.GetEntries())
	ui.Run()
}

func init() {
	addExtraNodeConfigurationCmd(dryrun.BuildCmd("dry-run", NodeConfigurationContextInjector,
		BuildNodeConfigurationSetConfigurationCmd(),
		BuildNodeConfigurationSetDefaultConfigurationCmd(),
		BuildNodeConfigurationResetConfigurationCmd(),
		BuildNodeConfigurationResetDefaultConfigurationCmd(),
	))
	addExtraNodeConfigurationCmd(rollback.BuildCmd("rollback", NodeConfigurationContextInjector))
}

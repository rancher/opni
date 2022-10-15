package test

import (
	"context"
	"runtime"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"github.com/rancher/opni/plugins/example/pkg/example"
	logging_agent "github.com/rancher/opni/plugins/logging/pkg/agent"
	logging_gateway "github.com/rancher/opni/plugins/logging/pkg/gateway"
	metrics_agent "github.com/rancher/opni/plugins/metrics/pkg/agent"
	metrics_gateway "github.com/rancher/opni/plugins/metrics/pkg/gateway"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

type apiextensionTestPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	server  apiextensions.ManagementAPIExtensionServer
	svcDesc *grpc.ServiceDesc
	impl    interface{}
}

func (p *apiextensionTestPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	s.RegisterService(p.svcDesc, p.impl)
	apiextensions.RegisterManagementAPIExtensionServer(s, p.server)
	return nil
}

func (p *apiextensionTestPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	return apiextensions.NewManagementAPIExtensionClient(c), nil
}

func NewApiExtensionTestPlugin(
	srv apiextensions.ManagementAPIExtensionServer,
	svcDesc *grpc.ServiceDesc,
	impl interface{},
) *plugin.ClientConfig {
	p := &apiextensionTestPlugin{
		server:  srv,
		svcDesc: svcDesc,
		impl:    impl,
	}
	scheme := meta.NewScheme()
	scheme.Add(managementext.ManagementAPIExtensionPluginID, p)

	cfg := plugins.ServeConfig(scheme)
	ch := make(chan *plugin.ReattachConfig)
	cfg.Test = &plugin.ServeTestConfig{
		ReattachConfigCh: ch,
	}
	go plugin.Serve(cfg)

	return &plugin.ClientConfig{
		HandshakeConfig: plugins.Handshake,
		Plugins:         scheme.PluginMap(),
		Reattach:        <-ch,
		Managed:         true,
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Error,
		}),
	}
}

type testPlugin struct {
	Scheme   meta.Scheme
	Metadata meta.PluginMeta
}

func LoadPlugins(loader *plugins.PluginLoader, mode meta.PluginMode) int {
	var metricsPluginScheme, loggingPluginScheme meta.Scheme
	var scheme meta.Scheme
	switch mode {
	case meta.ModeGateway:
		scheme = plugins.GatewayScheme
		metricsPluginScheme = metrics_gateway.Scheme(context.Background())
		loggingPluginScheme = logging_gateway.Scheme(context.Background())
	case meta.ModeAgent:
		scheme = plugins.AgentScheme
		metricsPluginScheme = metrics_agent.Scheme(context.Background())
		loggingPluginScheme = logging_agent.Scheme(context.Background())
	default:
		panic("unknown plugin mode: " + mode)
	}

	testPlugins := []testPlugin{
		{
			Scheme: metricsPluginScheme,
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_metrics",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/metrics",
			},
		},
		{
			Scheme: loggingPluginScheme,
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_logging",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/logging",
			},
		},
		{
			Scheme: example.Scheme(context.Background()),
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_example",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/example",
			},
		},
		{
			Scheme: slo.Scheme(context.Background()),
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_slo",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/slo",
			},
		},
		{
			Scheme: alerting.Scheme(context.Background()),
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_alerting",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/alerting",
			},
		},
	}
	wg := &sync.WaitGroup{}
	for _, p := range testPlugins {
		p := p
		sc := plugins.ServeConfig(p.Scheme)
		ch := make(chan *plugin.ReattachConfig, 1)
		sc.Test = &plugin.ServeTestConfig{
			ReattachConfigCh: ch,
		}
		go plugin.Serve(sc)
		rc := <-ch
		cc := plugins.ClientConfig(p.Metadata, scheme, rc)
		wg.Add(1)
		go func() {
			defer wg.Done()
			loader.LoadOne(context.Background(), p.Metadata, cc)
		}()
	}
	wg.Wait()
	loader.Complete()
	return len(testPlugins)
}

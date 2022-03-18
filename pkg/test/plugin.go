package test

import (
	"context"
	"runtime"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	managementext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/cortex"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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
		Logger:          logger.NewHCLogger(logger.New(logger.WithLogLevel(zap.WarnLevel))).Named("plugin"),
	}
}

type testPlugin struct {
	Scheme   meta.Scheme
	Metadata meta.PluginMeta
}

func LoadPlugins(loader *plugins.PluginLoader) int {
	testPlugins := []testPlugin{
		{
			Scheme: cortex.Scheme(context.Background()),
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_cortex",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni-monitoring/plugins/cortex",
			},
		},
	}
	for _, p := range testPlugins {
		sc := plugins.ServeConfig(p.Scheme)
		ch := make(chan *plugin.ReattachConfig, 1)
		sc.Test = &plugin.ServeTestConfig{
			ReattachConfigCh: ch,
		}
		go plugin.Serve(sc)
		rc := <-ch
		cc := plugins.ClientConfig(p.Metadata, plugins.ClientScheme, rc)
		loader.Load(p.Metadata, cc)
	}
	return len(testPlugins)
}

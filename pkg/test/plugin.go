package test

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	managementext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
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

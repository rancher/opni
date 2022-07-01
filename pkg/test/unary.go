package test

import (
	"context"
	"runtime"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	gatewayunaryext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/unary"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/example/pkg/example"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type apiextensionUnaryTestPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	server  apiextensions.UnaryAPIExtensionServer
	svcDesc *grpc.ServiceDesc
	impl    interface{}
}

func (p *apiextensionUnaryTestPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterUnaryAPIExtensionServer(s, p.server)
	s.RegisterService(p.svcDesc, p.impl)
	return nil
}

func (p *apiextensionUnaryTestPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	return apiextensions.NewUnaryAPIExtensionClient(c), nil
}

func NewApiExtensionUnaryTestPlugin(
	srv apiextensions.UnaryAPIExtensionServer,
	svcDesc *grpc.ServiceDesc,
	impl interface{},
) *plugin.ClientConfig {
	p := &apiextensionUnaryTestPlugin{
		server:  srv,
		svcDesc: svcDesc,
		impl:    impl,
	}
	scheme := meta.NewScheme()
	scheme.Add(gatewayunaryext.UnaryAPIExtensionPluginID, p)

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

type testUnaryPlugin struct {
	Scheme   meta.Scheme
	Metadata meta.PluginMeta
}

func LoadUnaryPlugins(loader *plugins.PluginLoader) int {
	testPlugins := []testPlugin{
		{
			Scheme: example.Scheme(),
			Metadata: meta.PluginMeta{
				BinaryPath: "plugin_example",
				GoVersion:  runtime.Version(),
				Module:     "github.com/rancher/opni/plugins/example",
			},
		},
	}
	wg := &sync.WaitGroup{}
	for _, p := range testPlugins {
		sc := plugins.ServeConfig(p.Scheme)
		ch := make(chan *plugin.ReattachConfig, 1)
		sc.Test = &plugin.ServeTestConfig{
			ReattachConfigCh: ch,
		}
		go plugin.Serve(sc)
		rc := <-ch
		cc := plugins.ClientConfig(p.Metadata, plugins.ClientScheme, rc)
		wg.Add(1)
		p := p
		go func() {
			defer wg.Done()
			loader.LoadOne(context.Background(), p.Metadata, cc)
		}()
	}
	wg.Wait()
	loader.Complete()
	return len(testPlugins)
}

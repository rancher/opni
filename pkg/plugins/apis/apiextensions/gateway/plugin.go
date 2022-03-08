package gatewayext

import (
	"context"
	"crypto/tls"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"google.golang.org/grpc"
)

const GatewayAPIExtensionPluginID = "apiextensions.GatewayAPIExtension"

// Plugin side

// internal RPC
type GatewayAPIExtensionServer interface {
	HttpAddr() int
	ConfigureRoutes() [][]*fiber.Route
}

// user-facing
type GatewayAPIExtension interface {
	ConfigureRoutes(*fiber.App)
}

type gatewayApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	apiextensions.UnimplementedGatewayAPIExtensionServer

	app  *fiber.App
	impl GatewayAPIExtension
}

var _ plugin.Plugin = (*gatewayApiExtensionPlugin)(nil)
var _ plugin.Plugin = (*gatewayApiExtensionPlugin)(nil)

// Plugin side
func (p *gatewayApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	p.app = fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	apiextensions.RegisterGatewayAPIExtensionServer(s, p)
	return nil
}

func (p *gatewayApiExtensionPlugin) Configure(
	ctx context.Context,
	certCfg *apiextensions.CertConfig,
) (*apiextensions.GatewayAPIExtensionConfig, error) {
	tlsCfg, err := certCfg.TLSConfig()
	if err != nil {
		return nil, err
	}
	listener, err := tls.Listen("tcp4", "127.0.0.1:0", tlsCfg)
	if err != nil {
		return nil, err
	}
	p.impl.ConfigureRoutes(p.app)
	go func() {
		if err := p.app.Listener(listener); err != nil {
			panic(err)
		}
	}()
	return &apiextensions.GatewayAPIExtensionConfig{
		HttpAddr: listener.Addr().String(),
		Routes:   stackToRoutes(p.app.Stack()),
	}, nil
}

func stackToRoutes(stack [][]*fiber.Route) []*apiextensions.Route {
	list := make([]*apiextensions.Route, 0)
	for _, routes := range stack {
		for _, route := range routes {
			list = append(list, &apiextensions.Route{
				Method: route.Method,
				Name:   route.Name,
				Path:   route.Path,
				Params: route.Params,
			})
		}
	}
	return list
}

// Gateway side
func (p *gatewayApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	return apiextensions.NewGatewayAPIExtensionClient(c), nil
}

func NewPlugin(impl GatewayAPIExtension) plugin.Plugin {
	return &gatewayApiExtensionPlugin{
		impl: impl,
	}
}

func init() {
	plugins.ClientScheme.Add(GatewayAPIExtensionPluginID, NewPlugin(nil))
}

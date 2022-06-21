package gatewayext

import (
	"context"
	"crypto/tls"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"google.golang.org/grpc"
)

const (
	GatewayAPIExtensionPluginID = "opni.apiextensions.GatewayAPIExtension"
	ServiceID                   = "apiextensions.GatewayAPIExtension"
)

// Plugin side
type GatewayAPIExtension interface {
	ConfigureRoutes(*gin.Engine)
}

type gatewayApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	apiextensions.UnimplementedGatewayAPIExtensionServer

	router *gin.Engine
	impl   GatewayAPIExtension
}

var _ plugin.Plugin = (*gatewayApiExtensionPlugin)(nil)

// Plugin side
func (p *gatewayApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	p.router = gin.New()

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
	p.router.Use(otelgin.Middleware("gateway-api"))
	p.impl.ConfigureRoutes(p.router)

	go func() {
		if err := p.router.RunListener(listener); err != nil {
			panic(err)
		}
	}()
	routes := []*apiextensions.RouteInfo{}
	for _, rt := range p.router.Routes() {
		routes = append(routes, &apiextensions.RouteInfo{
			Path:   rt.Path,
			Method: rt.Method,
		})
	}
	return &apiextensions.GatewayAPIExtensionConfig{
		HttpAddr: listener.Addr().String(),
		Routes:   routes,
	}, nil
}

// Gateway side
func (p *gatewayApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
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

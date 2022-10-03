package http

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-plugin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"google.golang.org/grpc"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
)

const (
	HTTPAPIExtensionPluginID = "opni.apiextensions.HTTPAPIExtension"
	ServiceID                = "apiextensions.HTTPAPIExtension"
)

type HTTPAPIExtension interface {
	ConfigureRoutes(*gin.Engine)
}

type httpApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	apiextensions.UnimplementedHTTPAPIExtensionServer

	router *gin.Engine
	impl   HTTPAPIExtension
}

var _ plugin.Plugin = (*httpApiExtensionPlugin)(nil)

// Plugin side
func (p *httpApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	p.router = gin.New()

	apiextensions.RegisterHTTPAPIExtensionServer(s, p)
	return nil
}

func (p *httpApiExtensionPlugin) Configure(
	ctx context.Context,
	certCfg *apiextensions.CertConfig,
) (*apiextensions.HTTPAPIExtensionConfig, error) {
	var listener net.Listener
	var err error
	if certCfg.Insecure {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
	} else {
		tlsCfg, err := certCfg.TLSConfig()
		if err != nil {
			return nil, err
		}
		listener, err = tls.Listen("tcp4", "127.0.0.1:0", tlsCfg)
		if err != nil {
			return nil, err
		}
	}
	p.router.Use(otelgin.Middleware("http-api"))
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
	return &apiextensions.HTTPAPIExtensionConfig{
		HttpAddr: listener.Addr().String(),
		Routes:   routes,
	}, nil
}

// Server side
func (p *httpApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewHTTPAPIExtensionClient(c), nil
}

func NewPlugin(impl HTTPAPIExtension) plugin.Plugin {
	return &httpApiExtensionPlugin{
		impl: impl,
	}
}

func init() {
	plugins.GatewayScheme.Add(HTTPAPIExtensionPluginID, NewPlugin(nil))
	plugins.AgentScheme.Add(HTTPAPIExtensionPluginID, NewPlugin(nil))
}

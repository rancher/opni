package gatewayext

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/dghubble/trie"
	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"google.golang.org/grpc"
)

const (
	GatewayAPIExtensionPluginID = "opni.apiextensions.GatewayAPIExtension"
	ServiceID                   = "apiextensions.GatewayAPIExtension"
)

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
		StrictRouting:         true,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           10 * time.Second,
	})
	logger.ConfigureAppLogger(p.app, "gateway-ext")
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
		HttpAddr:     listener.Addr().String(),
		PathPrefixes: findPathPrefixes(p.app.Stack()),
	}, nil
}

func findPathPrefixes(stack [][]*fiber.Route) []string {
	t := trie.NewPathTrie()
	for _, routesByMethod := range stack {
		for _, route := range routesByMethod {
			t.Put(route.Path, route)
		}
	}
	topLevelPaths := []string{}
	t.Walk(func(key string, value interface{}) error {
		for _, p := range topLevelPaths {
			if strings.HasPrefix(key, p) {
				return nil
			}
		}
		topLevelPaths = append(topLevelPaths, key)
		return nil
	})
	return topLevelPaths
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

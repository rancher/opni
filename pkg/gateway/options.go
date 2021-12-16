package gateway

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-gateway/pkg/auth"
)

type GatewayOptions struct {
	listenAddr       string
	prefork          bool
	enableMonitor    bool
	trustedProxies   []string
	fiberMiddlewares []FiberMiddleware
	authMiddleware   auth.NamedMiddleware
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) Apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddr(addr string) GatewayOption {
	return func(o *GatewayOptions) {
		o.listenAddr = addr
	}
}

func WithPrefork(prefork bool) GatewayOption {
	return func(o *GatewayOptions) {
		o.prefork = prefork
	}
}

func WithTrustedProxies(proxies []string) GatewayOption {
	return func(o *GatewayOptions) {
		o.trustedProxies = proxies
	}
}

func WithMonitor(enableMonitor bool) GatewayOption {
	return func(o *GatewayOptions) {
		o.enableMonitor = enableMonitor
	}
}

type FiberMiddleware = func(*fiber.Ctx) error

func WithFiberMiddleware(middlewares ...FiberMiddleware) GatewayOption {
	return func(o *GatewayOptions) {
		o.fiberMiddlewares = append(o.fiberMiddlewares, middlewares...)
	}
}

func WithAuthMiddleware(name string) GatewayOption {
	return func(o *GatewayOptions) {
		var err error
		o.authMiddleware, err = auth.GetMiddleware(name)
		if err != nil {
			panic(err)
		}
	}
}

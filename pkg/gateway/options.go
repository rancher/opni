package gateway

import (
	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/config"
)

type GatewayOptions struct {
	prefork          bool
	fiberMiddlewares []FiberMiddleware
	authMiddleware   auth.NamedMiddleware
	lifecycler       config.Lifecycler
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) Apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithPrefork(prefork bool) GatewayOption {
	return func(o *GatewayOptions) {
		o.prefork = prefork
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

func WithLifecycler(lc config.Lifecycler) GatewayOption {
	return func(o *GatewayOptions) {
		o.lifecycler = lc
	}
}

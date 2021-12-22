package gateway

import (
	"crypto/tls"
	"crypto/x509"

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
	rootCA           *x509.Certificate
	keypair          *tls.Certificate
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

func WithKeypair(keypair *tls.Certificate) GatewayOption {
	return func(o *GatewayOptions) {
		o.keypair = keypair
	}
}

func WithRootCA(rootCA *x509.Certificate) GatewayOption {
	return func(o *GatewayOptions) {
		o.rootCA = rootCA
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

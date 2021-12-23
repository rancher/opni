package gateway

import (
	"crypto/tls"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-gateway/pkg/auth"
)

type GatewayOptions struct {
	httpListenAddr   string
	prefork          bool
	enableMonitor    bool
	trustedProxies   []string
	fiberMiddlewares []FiberMiddleware
	authMiddleware   auth.NamedMiddleware
	servingCert      *tls.Certificate
	managementSocket string
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) Apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddr(addr string) GatewayOption {
	return func(o *GatewayOptions) {
		o.httpListenAddr = addr
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

// Sets the certificate to be used for serving HTTPS requests. The certificate
// list must contain the full chain, including root CA.
func WithServingCert(cert *tls.Certificate) GatewayOption {
	return func(o *GatewayOptions) {
		o.servingCert = cert
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

func WithManagementSocket(socket string) GatewayOption {
	return func(o *GatewayOptions) {
		o.managementSocket = socket
	}
}

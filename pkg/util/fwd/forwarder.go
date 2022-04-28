package fwd

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/rancher/opni/pkg/logger"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type ForwarderOptions struct {
	logger    *zap.SugaredLogger
	tlsConfig *tls.Config
	name      string
}

type ForwarderOption func(*ForwarderOptions)

func (o *ForwarderOptions) Apply(opts ...ForwarderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogger(logger *zap.SugaredLogger) ForwarderOption {
	return func(o *ForwarderOptions) {
		o.logger = logger
	}
}

func WithName(name string) ForwarderOption {
	return func(o *ForwarderOptions) {
		o.name = strings.TrimSpace(name) + " "
	}
}

func WithTLS(tlsConfig *tls.Config) ForwarderOption {
	return func(o *ForwarderOptions) {
		o.tlsConfig = tlsConfig
	}
}

func To(addr string, opts ...ForwarderOption) func(*fiber.Ctx) error {
	defaultLogger := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("fwd")
	options := &ForwarderOptions{
		logger: defaultLogger,
	}
	options.Apply(opts...)

	if options.name != "" {
		options.logger = options.logger.Named(options.name)
	}

	hostClient := &fasthttp.HostClient{
		MaxConns:                 1024 * 8,
		ReadTimeout:              10 * time.Second,
		WriteTimeout:             10 * time.Second,
		NoDefaultUserAgentHeader: true,
		DisablePathNormalizing:   true,
		Addr:                     addr,
		IsTLS:                    options.tlsConfig != nil,
		TLSConfig:                options.tlsConfig,
	}

	return func(c *fiber.Ctx) error {
		forwardedFor := c.IP()
		forwardedHost := c.Hostname()
		forwardedProto := c.Protocol()
		options.logger.With(
			"method", c.Method(),
			"path", c.Path(),
			"to", addr,
			"for", forwardedFor,
			"host", forwardedHost,
		).Debugf("=>")

		req := c.Request()
		resp := c.Response()
		req.Header.Del(fiber.HeaderConnection)
		req.Header.Set(fiber.HeaderXForwardedFor, forwardedFor)
		req.Header.Set(fiber.HeaderXForwardedHost, forwardedHost)
		req.Header.Set(fiber.HeaderXForwardedProto, forwardedProto)
		if hostClient.IsTLS {
			req.Header.Set(fiber.HeaderXForwardedSsl, "on")
		}

		req.SetRequestURI(utils.UnsafeString(req.RequestURI()))
		if err := hostClient.Do(req, resp); err != nil {
			options.logger.With(
				zap.Error(err),
				"req", c.Path(),
			).Error("error forwarding request")
			return fmt.Errorf("error forwarding request: %w", err)
		}
		resp.Header.Del(fiber.HeaderConnection)
		if resp.StatusCode()/100 >= 4 {
			options.logger.With(
				"req", c.Path(),
				"status", resp.StatusCode(),
			).Info("server replied with error")
		}
		return nil
	}
}

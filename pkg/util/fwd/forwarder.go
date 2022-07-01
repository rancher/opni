package fwd

import (
	"bufio"
	"crypto/tls"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

type ForwarderOptions struct {
	logger    *zap.SugaredLogger
	tlsConfig *tls.Config
	name      string
	destHint  string
}

type ForwarderOption func(*ForwarderOptions)

func (o *ForwarderOptions) apply(opts ...ForwarderOption) {
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
		o.name = strings.TrimSpace(name)
	}
}

func WithTLS(tlsConfig *tls.Config) ForwarderOption {
	return func(o *ForwarderOptions) {
		o.tlsConfig = tlsConfig
	}
}

func WithDestHint(hint string) ForwarderOption {
	return func(o *ForwarderOptions) {
		o.destHint = hint
	}
}

func To(addr string, opts ...ForwarderOption) gin.HandlerFunc {
	defaultLogger := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("fwd")
	options := &ForwarderOptions{
		logger: defaultLogger,
	}
	options.apply(opts...)

	if options.name != "" {
		options.logger = options.logger.Named(options.name)
	}

	transport := otelhttp.NewTransport(&http.Transport{
		TLSClientConfig: options.tlsConfig,
	})

	tlsEnabled := options.tlsConfig != nil

	return func(c *gin.Context) {
		if tlsEnabled {
			c.Request.URL.Scheme = "https"
		} else {
			c.Request.URL.Scheme = "http"
		}
		c.Request.URL.Host = addr

		forwardedFor := c.RemoteIP()
		forwardedHost := c.Request.Host
		forwardedProto := c.Request.Proto
		to := addr
		if options.destHint != "" {
			to += " (" + options.destHint + ")"
		}
		options.logger.With(
			"method", c.Request.Method,
			"path", c.FullPath(),
			"to", to,
			"for", forwardedFor,
			"host", forwardedHost,
			"scheme", c.Request.URL.Scheme,
		).Debugf("=>")

		c.Header("X-Forwarded-For", forwardedFor)
		c.Header("X-Forwarded-Host", forwardedHost)
		c.Header("X-Forwarded-Proto", forwardedProto)
		if options.tlsConfig != nil {
			c.Header("X-Forwarded-Ssl", "on")
		}

		resp, err := transport.RoundTrip(c.Request)
		if err != nil {
			options.logger.With(
				zap.Error(err),
				"req", c.FullPath(),
			).Error("error forwarding request")
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				c.Header(k, v)
			}
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 >= 4 {
			options.logger.With(
				"req", c.FullPath(),
				"status", resp.StatusCode,
			).Info("server replied with error")
		}
		bufio.NewReader(resp.Body).WriteTo(c.Writer)
	}
}

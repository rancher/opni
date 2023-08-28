package dashboard

import (
	"context"
	"crypto/tls"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/web"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type Server struct {
	ServerOptions
	config *v1beta1.ManagementSpec
	logger *zap.SugaredLogger
}

type extraHandler struct {
	method  string
	prefix  string
	handler []gin.HandlerFunc
}

type ServerOptions struct {
	extraHandlers []extraHandler
	assetsFS      fs.FS
}

type ServerOption func(*ServerOptions)

func (o *ServerOptions) apply(opts ...ServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithHandler(method, prefix string, handler ...gin.HandlerFunc) ServerOption {
	return func(o *ServerOptions) {
		o.extraHandlers = append(o.extraHandlers, extraHandler{
			method:  method,
			prefix:  prefix,
			handler: handler,
		})
	}
}

func WithAssetsFS(fs fs.FS) ServerOption {
	return func(o *ServerOptions) {
		o.assetsFS = fs
	}
}

func NewServer(config *v1beta1.ManagementSpec, opts ...ServerOption) (*Server, error) {
	options := ServerOptions{
		assetsFS: web.DistFS,
	}
	options.apply(opts...)

	if !web.EmbeddedAssetsAvailable(options.assetsFS) {
		return nil, errors.New("embedded assets not available")
	}

	if config.WebListenAddress == "" {
		return nil, errors.New("management.webListenAddress not set in config")
	}
	return &Server{
		ServerOptions: options,
		config:        config,
		logger:        logger.New().Named("dashboard"),
	}, nil
}

func (ws *Server) ListenAndServe(ctx waitctx.RestrictiveContext) error {
	lg := ws.logger
	var listener net.Listener
	if ws.config.WebCerts != nil {
		certs, caPool, err := util.LoadServingCertBundle(*ws.config.WebCerts)
		if err != nil {
			return err
		}
		listener, err = tls.Listen("tcp4", ws.config.WebListenAddress, &tls.Config{
			Certificates: []tls.Certificate{*certs},
			ClientCAs:    caPool,
		})
		if err != nil {
			return err
		}
	} else {
		var err error
		listener, err = net.Listen("tcp4", ws.config.WebListenAddress)
		if err != nil {
			return err
		}
	}
	lg.With(
		"address", listener.Addr(),
	).Info("ui server starting")
	// proxyTracer := otel.Tracer("dashboard-proxy")
	webFsTracer := otel.Tracer("webfs")
	router := gin.New()
	router.Use(
		gin.Recovery(),
		gin.Logger(),
		logger.GinLogger(ws.logger),
		otelgin.Middleware("opni-ui"),
	)

	// Static assets
	sub, err := fs.Sub(ws.assetsFS, "dist")
	if err != nil {
		return err
	}

	webfs := http.FS(sub)

	router.NoRoute(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		_, span := webFsTracer.Start(ctx, c.Request.URL.Path)
		defer span.End()
		path := c.Request.URL.Path
		if path[0] == '/' {
			path = path[1:]
		}
		if _, err := fs.Stat(sub, path); err == nil {
			c.FileFromFS(path, webfs)
			return
		}

		c.FileFromFS("/", webfs) // serve index.html
	})

	opniApiAddr := ws.config.HTTPListenAddress
	mgmtUrl, err := url.Parse("http://" + opniApiAddr)
	if err != nil {
		lg.With(
			"url", opniApiAddr,
			"error", err,
		).Panic("failed to parse management API URL")
		return err
	}

	router.Any("/opni-api/*any", gin.WrapH(http.StripPrefix("/opni-api", httputil.NewSingleHostReverseProxy(mgmtUrl))))
	// router.Any("/opni-api/*any", gin.WrapH(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
	// 	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	// 	_, span := proxyTracer.Start(
	// 		ctx,
	// 		r.URL.Path,
	// 		func() (tr []trace.SpanStartOption) {
	// 			for k, v := range r.Header {
	// 				tr = append(tr, trace.WithAttributes(attribute.String(k, strings.Join(v, ","))))

	// 			}
	// 			tr = append(tr, trace.WithAttributes(attribute.String("proxy-method", r.Method)))
	// 			tr = append(tr, trace.WithAttributes(attribute.String("proxy-url", mgmtUrl.String())))
	// 			return tr
	// 		}()...,
	// 	)

	// 	defer func() {
	// 		cancel()
	// 		span.End()
	// 	}()

	// 	// round-trip to the management API
	// 	// strip the prefix /opni-api/
	// 	u := *mgmtUrl
	// 	u.Path = r.URL.Path[len("/opni-api/"):]

	// 	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), r.Body)
	// 	if err != nil {
	// 		lg.With(
	// 			zap.Error(err),
	// 		).Error("failed to create request")
	// 		rw.WriteHeader(http.StatusInternalServerError)
	// 		return
	// 	}
	// 	req.Header = r.Header
	// 	resp, err := http.DefaultClient.Do(req)
	// 	if err != nil {
	// 		lg.With(
	// 			zap.Error(err),
	// 		).Error("failed to round-trip management api request")
	// 		if errors.Is(err, ctx.Err()) {
	// 			rw.WriteHeader(http.StatusGatewayTimeout)
	// 			return
	// 		}
	// 		rw.WriteHeader(http.StatusInternalServerError)
	// 		return
	// 	}
	// 	defer resp.Body.Close()
	// 	for k, v := range resp.Header {
	// 		for _, vv := range v {
	// 			rw.Header().Add(k, vv)
	// 		}
	// 	}
	// 	rw.WriteHeader(resp.StatusCode)
	// 	io.Copy(rw, resp.Body)
	// })))

	for _, h := range ws.extraHandlers {
		router.Handle(h.method, h.prefix, h.handler...)
	}

	return util.ServeHandler(ctx, router, listener)
}

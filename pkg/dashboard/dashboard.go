package dashboard

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/auth/local"
	"github.com/rancher/opni/pkg/auth/middleware"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/types"
	proxyrouter "github.com/rancher/opni/pkg/proxy/router"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/web"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"golang.org/x/oauth2"
)

type Server struct {
	ServerOptions
	config      *v1beta1.ManagementSpec
	logger      *slog.Logger
	pl          plugins.LoaderInterface
	oauthConfig *oauth2.Config
	ds          AuthDataSource
}

type extraHandler struct {
	method  string
	prefix  string
	handler []gin.HandlerFunc
}

type ServerOptions struct {
	extraHandlers []extraHandler
	assetsFS      fs.FS
	authenticator local.LocalAuthenticator
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

func WithLocalAuthenticator(authenticator local.LocalAuthenticator) ServerOption {
	return func(o *ServerOptions) {
		o.authenticator = authenticator
	}
}

func NewServer(
	config *v1beta1.ManagementSpec,
	pl plugins.LoaderInterface,
	ds AuthDataSource,
	opts ...ServerOption,
) (*Server, error) {
	options := ServerOptions{
		assetsFS:      web.DistFS,
		authenticator: local.NewLocalAuthenticator(ds.StorageBackend().KeyValueStore(authutil.AuthNamespace)),
	}
	options.apply(opts...)

	if !web.WebAssetsAvailable(options.assetsFS) {
		return nil, errors.New("web assets not available")
	}

	if config.WebListenAddress == "" {
		return nil, errors.New("management.webListenAddress not set in config")
	}
	return &Server{
		ServerOptions: options,
		config:        config,
		logger:        logger.New().WithGroup("dashboard"),
		pl:            pl,
		ds:            ds,
	}, nil
}

func (ws *Server) ListenAndServe(ctx context.Context) error {
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
	webFsTracer := otel.Tracer("webfs")
	router := gin.New()
	router.Use(
		gin.Recovery(),
		logger.GinLogger(ws.logger),
		otelgin.Middleware("opni-ui"),
	)
	middleware := ws.configureAuth(ctx, router)

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
		).Error("failed to parse management API URL")
		panic("failed to parse management API URL")
	}
	apiGroup := router.Group("/opni-api")
	apiGroup.Use(middleware.Handler())
	apiGroup.Any("/*any", gin.WrapH(http.StripPrefix("/opni-api", httputil.NewSingleHostReverseProxy(mgmtUrl))))

	proxy := router.Group("/proxy")
	proxy.Use(middleware.Handler())
	ws.pl.Hook(hooks.OnLoad(func(p types.ProxyPlugin) {
		log := lg.WithGroup("proxy")
		pluginRouter, err := proxyrouter.NewRouter(proxyrouter.RouterConfig{
			Store:  ws.ds.StorageBackend(),
			Logger: log,
			Client: p,
		})
		if err != nil {
			log.With(
				logger.Err(err),
			).Error("failed to create plugin router")
			return
		}
		pluginRouter.SetRoutes(proxy)
	}))

	for _, h := range ws.extraHandlers {
		router.Handle(h.method, h.prefix, h.handler...)
	}

	return util.ServeHandler(ctx, router, listener)
}

func (ws *Server) configureAuth(ctx context.Context, router *gin.Engine) *middleware.MultiMiddleware {
	issuer := ""             // TODO: Load this from config
	clientID := ""           // TODO: Load this from config
	clientSecret := ""       //TODO: Load this from config
	scopes := []string{}     //TODO: Load this from config
	localServerAddress := "" // TODO: Load this from config
	var useOIDC bool         //TODO: Load this from config

	if useOIDC {
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			panic(err)
		}
		ws.oauthConfig = &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
			RedirectURL:  fmt.Sprintf("%s/auth/oidc/callback", localServerAddress),
		}
		handler := oidcHandler{
			logger:   ws.logger.WithGroup("oidc_auth"),
			config:   ws.oauthConfig,
			provider: provider,
		}
		router.Any("/auth/oidc/redirect", handler.handleRedirect)
		router.Any("/auth/oidc/callback", handler.handleCallback)
	}

	middleware := &middleware.MultiMiddleware{
		Logger:             ws.logger.WithGroup("auth_middleware"),
		Config:             ws.oauthConfig,
		IdentifyingClaim:   "user", //TODO: load this from config
		UseOIDC:            useOIDC,
		LocalAuthenticator: ws.authenticator,
	}
	router.GET("/auth/type", middleware.GetAuthType)
	return middleware
}

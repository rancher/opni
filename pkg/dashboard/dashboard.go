package dashboard

import (
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/web"
	"go.uber.org/zap"
)

type ExtraHandler struct {
	Path    string
	Handler http.HandlerFunc
}

var (
	ExtraHandlers = []ExtraHandler{}
)

func AddExtraHandler(path string, handler http.HandlerFunc) {
	ExtraHandlers = append(ExtraHandlers, ExtraHandler{
		Path:    path,
		Handler: handler,
	})
}

type Server struct {
	config *v1beta1.GatewayConfig
	logger *zap.SugaredLogger
}

func NewServer(config *v1beta1.GatewayConfig) (*Server, error) {
	if !web.EmbeddedAssetsAvailable() {
		return nil, errors.New("embedded assets not available")
	}
	if config.Spec.Management.WebListenAddress == "" {
		return nil, errors.New("management.webListenAddress not set in config")
	}
	return &Server{
		config: config,
		logger: logger.New().Named("dashboard"),
	}, nil
}

func (ws *Server) ListenAndServe(ctx waitctx.RestrictiveContext) error {
	lg := ws.logger
	listener, err := net.Listen("tcp4", ws.config.Spec.Management.WebListenAddress)
	if err != nil {
		return err
	}
	lg.With(
		"address", listener.Addr(),
	).Info("ui server starting")

	router := gin.New()
	router.Use(gin.Recovery(), logger.GinLogger(ws.logger))

	// Static assets
	sub, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return err
	}

	webfs := http.FS(sub)

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path[0] == '/' {
			path = path[1:]
		}
		if _, err := fs.Stat(sub, path); err == nil {
			c.FileFromFS(path, webfs)
			return
		}

		c.FileFromFS("200.html", webfs)
	})

	opniApiAddr := ws.config.Spec.Management.HTTPListenAddress
	mgmtUrl, err := url.Parse("http://" + opniApiAddr)
	if err != nil {
		lg.With(
			"url", opniApiAddr,
			"error", err,
		).Panic("failed to parse management API URL")
		return err
	}
	router.Any("/opni-api/*any", gin.WrapH(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// round-trip to the management API
		// strip the prefix /opni-api/
		u := *mgmtUrl
		u.Path = r.URL.Path[len("/opni-api/"):]

		req, err := http.NewRequest(r.Method, u.String(), r.Body)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to create request")
			rw.WriteHeader(500)
			return
		}
		req.Header = r.Header
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to round-trip management api request")
			rw.WriteHeader(500)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			for _, vv := range v {
				rw.Header().Add(k, vv)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		io.Copy(rw, resp.Body)
	})))

	return util.ServeHandler(ctx, router, listener)
}

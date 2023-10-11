package alerting

import (
	"net/http"

	"github.com/gin-contrib/pprof"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
)

var _ httpext.HTTPAPIExtension = (*Plugin)(nil)

func (p *Plugin) ConfigureRoutes(router *gin.Engine) {
	router.Use(logger.GinLogger(p.logger.With("component", "http-proxy")), gin.Recovery())
	p.hsServer.ConfigureRoutes(router)
	p.httpProxy.ConfigureRoutes(router)
}

type healthStatusServer struct {
	readyFunc   func() error
	healthyFunc func() error
}

func newHealthStatusServer(
	readyFunc func() error,
	healthyFunc func() error,
) *healthStatusServer {
	return &healthStatusServer{
		readyFunc:   readyFunc,
		healthyFunc: healthyFunc,
	}
}

var _ httpext.HTTPAPIExtension = (*healthStatusServer)(nil)

func (h *healthStatusServer) ConfigureRoutes(router *gin.Engine) {
	router.GET("/plugin_alerting/ready", h.ready)
	router.GET("/plugin_alerting/healthy", h.healthy)
	pprof.Register(router, "/debug/plugin_alerting/pprof")
}

func (h *healthStatusServer) ready(c *gin.Context) {
	if err := h.readyFunc(); err != nil {
		c.Status(http.StatusServiceUnavailable)
	}
	c.Status(http.StatusOK)
}

func (h *healthStatusServer) healthy(c *gin.Context) {
	if err := h.healthyFunc(); err != nil {
		c.Status(http.StatusServiceUnavailable)
	}
	c.Status(http.StatusOK)
}

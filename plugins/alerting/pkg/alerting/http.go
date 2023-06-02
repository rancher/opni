package alerting

import (
	"context"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"go.uber.org/zap"
)

const proxyPath = "/plugin_alerting/alertmanager"

type HttpApiServer struct {
	lg *zap.SugaredLogger

	readyFunc   func() error
	healthyFunc func() error
	client      client.AlertingClient
}

func NewHttpApiServer(
	lg *zap.SugaredLogger,
	client client.AlertingClient,
	readyFunc func() error,
	healthyFunc func() error,
) *HttpApiServer {
	return &HttpApiServer{
		lg:          lg,
		readyFunc:   readyFunc,
		healthyFunc: healthyFunc,
		client:      client,
	}
}

// func (h *HttpApiServer) configureProxy()

func (h *HttpApiServer) ready(c *gin.Context) {
	if err := h.readyFunc(); err != nil {
		c.Status(http.StatusServiceUnavailable)
	}
	c.Status(http.StatusOK)
}

func (h *HttpApiServer) healthy(c *gin.Context) {
	if err := h.healthyFunc(); err != nil {
		c.Status(http.StatusServiceUnavailable)
	}
	c.Status(http.StatusOK)
}

func (h *HttpApiServer) alertmanagerProxy(c *gin.Context) {
	ctx, ca := context.WithTimeout(c.Request.Context(), time.Second*5)
	defer ca()
	c.Request.URL.Path = c.Request.URL.Path[len(proxyPath):]
	c.Request.URL.Scheme = "http"
	resp, err := h.client.ProxyClient().Handle(ctx, c.Request)
	if err != nil {
		errorStatus := http.StatusInternalServerError
		if ctx.Err() != nil {
			errorStatus = http.StatusGatewayTimeout
		}
		c.AbortWithStatus(errorStatus)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
	}
	c.Data(resp.StatusCode, "application/json", data)
	for key, value := range resp.Header {
		for _, v := range value {
			c.Header(key, v)
		}
	}
}

func (h *HttpApiServer) ConfigureRoutes(router *gin.Engine) {
	router.Use(logger.GinLogger(h.lg), gin.Recovery())

	router.GET("/plugin_alerting/ready", h.ready)
	router.GET("/plugin_alerting/healthy", h.healthy)

	router.Any(path.Join(proxyPath, "/api/v2/alerts/*any"), h.alertmanagerProxy)
	router.Any(path.Join(proxyPath, "/api/v1/alerts/*any"), h.alertmanagerProxy)
	pprof.Register(router, "/debug/plugin_alerting/pprof")
}

var _ httpext.HTTPAPIExtension = (*HttpApiServer)(nil)

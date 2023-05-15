package alerting

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
)

var _ httpext.HTTPAPIExtension = (*Plugin)(nil)

func (p *Plugin) ConfigureRoutes(router *gin.Engine) {
	router.Use(logger.GinLogger(p.Logger), gin.Recovery())

	router.GET("/plugin_alerting/ready", p.ready)
	router.GET("/plugin_alerting/healthy", p.healthy)

	pprof.Register(router, "/debug/plugin_alerting/pprof")
}

func (p *Plugin) ready(c *gin.Context) {
	for _, comp := range p.Components() {
		if !comp.Ready() {
			c.Status(503)
			return
		}
	}
	c.Status(200)
}

func (p *Plugin) healthy(c *gin.Context) {
	for _, comp := range p.Components() {
		if !comp.Healthy() {
			c.Status(503)
			return
		}
	}

	// TODO : this should also check things like registered streams are up and running
	c.Status(200)
}

package main

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway"
)

func main() {
	tracing.Configure("plugin_cortex")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	switch meta.PluginMode(os.Getenv("OPNI_PLUGIN_MODE")) {
	case meta.ModeUnknown, meta.ModeGateway:
		plugins.Serve(gateway.Scheme(ctx))
	case meta.ModeAgent:
		plugins.Serve(agent.Scheme(ctx))
	}
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

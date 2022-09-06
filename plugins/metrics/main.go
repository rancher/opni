package main

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
)

func main() {
	tracing.Configure("plugin_cortex")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	switch os.Getenv("PLUGIN_MODE") {
	case "", "gateway":
		plugins.Serve(cortex.Scheme(ctx))
	case "agent":
		plugins.Serve(agent.Scheme(ctx))
	}
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
)

func main() {
	tracing.Configure("plugin_cortex")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(cortex.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

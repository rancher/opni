package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/example/pkg/example"
)

func main() {
	tracing.Configure("plugin_example")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(example.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

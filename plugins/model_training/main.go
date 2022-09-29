package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/model_training/pkg/model_training"
)

func main() {
	tracing.Configure("plugin_model_training")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(model_training.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

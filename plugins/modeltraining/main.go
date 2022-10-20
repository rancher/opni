package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	modeltraining "github.com/rancher/opni/plugins/modeltraining/pkg/modeltraining"
)

func main() {
	tracing.Configure("plugin_model_training")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(modeltraining.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

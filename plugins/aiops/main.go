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
	"github.com/rancher/opni/plugins/aiops/pkg/gateway"
)

func main() {
	tracing.Configure("plugin_modeltraining")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())

	switch meta.PluginMode(os.Getenv("OPNI_PLUGIN_MODE")) {
	case meta.ModeUnknown, meta.ModeGateway:
		tracing.Configure("plugin_logging_gateway")
		plugins.Serve(gateway.Scheme(ctx))
	}

	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

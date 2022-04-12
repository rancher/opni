package main

import (
	"context"
	"time"

	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/cortex"
)

func main() {
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(cortex.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

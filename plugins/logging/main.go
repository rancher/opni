package main

import (
	"context"
	"time"

	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
	"github.com/rancher/opni-monitoring/plugins/logging/pkg/logging"
)

func main() {
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(logging.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

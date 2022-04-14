package main

import (
	"context"
	"time"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/logging/pkg/logging"
)

func main() {
	ctx, ca := context.WithCancel(waitctx.Background())
	plugins.Serve(logging.Scheme(ctx))
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}

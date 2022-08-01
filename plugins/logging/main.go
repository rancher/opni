package main

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/logging/pkg/logging"
	"k8s.io/client-go/rest"
)

func main() {
	tracing.Configure("plugin_logging")
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	for {
		if err := runPlugin(ctx, ca); err != nil {
			panic(err)
		}
	}
}

func runPlugin(ctx context.Context, cancel context.CancelFunc) error {
	inCluster := true
	restconfig, err := rest.InClusterConfig()
	if err != nil {
		if errors.Is(err, rest.ErrNotInCluster) {
			inCluster = false
		}
		return err
	}
	var fCancel context.CancelFunc
	if inCluster {
		features.PopulateFeatures(ctx, restconfig)
		fCancel = features.FeatureList.WatchConfigMap()
	} else {
		fCancel = cancel
	}

	go plugins.Serve(logging.Scheme(ctx))

	reloadC := make(chan struct{})
	go func() {
		fNotify := make(<-chan struct{})
		if inCluster {
			fNotify = features.FeatureList.NotifyChange()
		}
		<-fNotify
		close(reloadC)
	}()

	<-reloadC
	fCancel()
	cancel()
	waitctx.Wait(ctx, 5*time.Second)
	return nil
}

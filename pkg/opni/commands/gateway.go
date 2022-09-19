package commands

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/management"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"

	_ "github.com/rancher/opni/pkg/plugins/apis"
)

func BuildGatewayCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	run := func() error {
		tracing.Configure("gateway")

		objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)

		ctx, cancel := context.WithCancel(waitctx.Background())

		inCluster := true
		restconfig, err := rest.InClusterConfig()
		if err != nil {
			if errors.Is(err, rest.ErrNotInCluster) {
				inCluster = false
			} else {
				lg.Fatalf("failed to create config: %s", err)
			}
		}

		var fCancel context.CancelFunc
		if inCluster {
			features.PopulateFeatures(ctx, restconfig)
			fCancel = features.FeatureList.WatchConfigMap()
		} else {
			fCancel = cancel
		}

		machinery.LoadAuthProviders(ctx, objects)
		var gatewayConfig *v1beta1.GatewayConfig
		found := objects.Visit(
			func(config *v1beta1.GatewayConfig) {
				if gatewayConfig == nil {
					gatewayConfig = config
				}
			},
			func(ap *v1beta1.AuthProvider) {
				// noauth is a special case, we need to start a noauth server but only
				// once - other auth provider consumers such as plugins can load
				// auth providers themselves, but we don't want them to start their
				// own noauth server.
				if ap.Name == "noauth" {
					server := machinery.NewNoauthServer(ctx, ap)
					waitctx.Go(ctx, func() {
						if err := server.ListenAndServe(ctx); err != nil {
							lg.With(
								zap.Error(err),
							).Warn("noauth server exited with error")
						}
					})
				}
			},
		)
		if !found {
			lg.Fatal("config file does not contain a GatewayConfig object")
		}

		lg.With(
			"dirs", gatewayConfig.Spec.Plugins.Dirs,
		).Info("loading plugins")
		pluginLoader := plugins.NewPluginLoader()

		lifecycler := config.NewLifecycler(objects)
		g := gateway.NewGateway(ctx, gatewayConfig, pluginLoader,
			gateway.WithLifecycler(lifecycler),
		)

		m := management.NewServer(ctx, &gatewayConfig.Spec.Management, g, pluginLoader,
			management.WithCapabilitiesDataSource(g),
			management.WithHealthStatusDataSource(g),
			management.WithLifecycler(lifecycler),
		)

		g.MustRegisterCollector(m)

		pluginLoader.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
			lg.Infof("loaded %d plugins", numLoaded)
		}))

		pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
			waitctx.AddOne(ctx)
			defer waitctx.Done(ctx)
			if err := m.ListenAndServe(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("management server exited with error")
			}
		}))

		pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
			waitctx.AddOne(ctx)
			defer waitctx.Done(ctx)
			if err := g.ListenAndServe(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("gateway server exited with error")
			}
		}))

		pluginLoader.LoadPlugins(ctx, gatewayConfig.Spec.Plugins, plugins.GatewayScheme)

		style := chalk.Yellow.NewStyle().
			WithBackground(chalk.ResetColor).
			WithTextStyle(chalk.Bold)
		reloadC := make(chan struct{})
		go func() {
			c, err := lifecycler.ReloadC()

			fNotify := make(<-chan struct{})
			if inCluster {
				fNotify = features.FeatureList.NotifyChange()
			}

			if err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("failed to get reload channel from lifecycler")
			}
			select {
			case <-c:
				lg.Info(style.Style("--- received reload signal ---"))
				close(reloadC)
			case <-fNotify:
				lg.Info(style.Style("--- received feature update signal ---"))
				close(reloadC)
			}
		}()

		<-reloadC
		lg.Info(style.Style("waiting for servers to shut down"))
		fCancel()
		cancel()
		waitctx.WaitWithTimeout(ctx, 60*time.Second, 10*time.Second)

		atomic.StoreUint32(&plugin.Killed, 0)
		lg.Info(style.Style("--- reloading ---"))
		return nil
	}

	serveCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the Opni Monitoring Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			defer waitctx.RecoverTimeout()
			for {
				if err := run(); err != nil {
					return err
				}
			}
		},
	}

	serveCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return serveCmd
}

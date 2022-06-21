package commands

import (
	"context"
	"sync/atomic"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
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

	// Import all plugin apis to ensure they are added to the client scheme
	_ "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	_ "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/stream"
	_ "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/unary"
	_ "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	_ "github.com/rancher/opni/pkg/plugins/apis/capability"
	_ "github.com/rancher/opni/pkg/plugins/apis/metrics"
	_ "github.com/rancher/opni/pkg/plugins/apis/system"
)

func BuildGatewayCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	run := func() error {
		tracing.Configure("gateway")

		objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)

		ctx, cancel := context.WithCancel(waitctx.Background())
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
					machinery.SetupNoauthServer(ctx, lg, ap)
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
			if err := m.ListenAndServe(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("management server exited with error")
			}
		}))

		pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
			if err := g.ListenAndServe(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Error("gateway server exited with error")
			}
		}))

		pluginLoader.LoadPlugins(ctx, gatewayConfig.Spec.Plugins)

		style := chalk.Yellow.NewStyle().
			WithBackground(chalk.ResetColor).
			WithTextStyle(chalk.Bold)
		reloadC := make(chan struct{})
		go func() {
			c, err := lifecycler.ReloadC()
			if err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("failed to get reload channel from lifecycler")
			}
			<-c
			lg.Info(style.Style("--- received reload signal ---"))
			cancel()
			close(reloadC)
		}()

		<-reloadC
		lg.Info(style.Style("waiting for servers to shut down"))
		waitctx.Wait(ctx)

		atomic.StoreUint32(&plugin.Killed, 0)
		lg.Info(style.Style("--- reloading ---"))
		return nil
	}

	serveCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the Opni Monitoring Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
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

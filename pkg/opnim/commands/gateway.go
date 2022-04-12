package commands

import (
	"context"
	"sync/atomic"

	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/gateway"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/machinery"
	"github.com/rancher/opni-monitoring/pkg/management"
	cliutil "github.com/rancher/opni-monitoring/pkg/opnim/util"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	gatewayext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/gateway"
	managementext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/metrics"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
)

func BuildGatewayCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	run := func() error {
		objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)

		ctx, cancel := context.WithCancel(waitctx.Background())
		machinery.LoadAuthProviders(ctx, objects)
		var gatewayConfig *v1beta1.GatewayConfig
		objects.Visit(
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

		lg.With(
			"dirs", gatewayConfig.Spec.Plugins.Dirs,
		).Info("loading plugins")
		pluginLoader := plugins.NewPluginLoader()
		numLoaded := machinery.LoadPlugins(pluginLoader, gatewayConfig.Spec.Plugins)
		lg.Infof("loaded %d plugins", numLoaded)
		mgmtExtensionPlugins := plugins.DispenseAllAs[apiextensions.ManagementAPIExtensionClient](
			pluginLoader, managementext.ManagementAPIExtensionPluginID)
		gatewayExtensionPlugins := plugins.DispenseAllAs[apiextensions.GatewayAPIExtensionClient](
			pluginLoader, gatewayext.GatewayAPIExtensionPluginID)
		systemPlugins := pluginLoader.DispenseAll(system.SystemPluginID)
		capBackendPlugins := plugins.DispenseAllAs[capability.BackendClient](
			pluginLoader, capability.CapabilityBackendPluginID)
		metricsPlugins := plugins.DispenseAllAs[prometheus.Collector](
			pluginLoader, metrics.MetricsPluginID)

		lifecycler := config.NewLifecycler(objects)
		g := gateway.NewGateway(ctx, gatewayConfig,
			gateway.WithSystemPlugins(systemPlugins),
			gateway.WithLifecycler(lifecycler),
			gateway.WithCapabilityBackendPlugins(capBackendPlugins),
			gateway.WithAPIServerOptions(
				gateway.WithAPIExtensions(gatewayExtensionPlugins),
				gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
				gateway.WithMetricsPlugins(metricsPlugins),
			),
		)

		m := management.NewServer(ctx, &gatewayConfig.Spec.Management, g,
			management.WithCapabilitiesDataSource(g),
			management.WithSystemPlugins(systemPlugins),
			management.WithAPIExtensions(mgmtExtensionPlugins),
			management.WithLifecycler(lifecycler),
		)

		g.MustRegisterCollector(m)

		go func() {
			if err := m.ListenAndServe(); err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("management server exited with error")
			}
		}()

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

		if err := g.ListenAndServe(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("gateway server exited with error")
			return err
		}

		<-reloadC
		lg.Info(style.Style("waiting for servers to shut down"))
		waitctx.Wait(ctx)

		auth.ResetMiddlewares()
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

//go:build !minimal && !cli

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/dashboard"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/update/noop"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/rest"

	_ "github.com/rancher/opni/pkg/oci/kubernetes"
	_ "github.com/rancher/opni/pkg/oci/noop"
	_ "github.com/rancher/opni/pkg/plugins/apis"
	_ "github.com/rancher/opni/pkg/storage/crds"
	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
)

func BuildGatewayCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	run := func(ctx context.Context) error {
		tracing.Configure("gateway")

		objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)

		inCluster := true
		restconfig, err := rest.InClusterConfig()
		if err != nil {
			if errors.Is(err, rest.ErrNotInCluster) {
				inCluster = false
			} else {
				lg.Error(fmt.Sprintf("failed to create config: %s", err))
				os.Exit(1)
			}
		}

		if inCluster {
			features.PopulateFeatures(ctx, restconfig)
			fCancel := features.FeatureList.WatchConfigMap()
			context.AfterFunc(ctx, fCancel)
		}

		machinery.LoadAuthProviders(ctx, objects)
		var gatewayConfig *v1beta1.GatewayConfig
		var noauthServer *noauth.Server
		found := objects.Visit(
			func(config *v1beta1.GatewayConfig) {
				if gatewayConfig == nil {
					gatewayConfig = config
				}
			},
			func(ap *v1beta1.AuthProvider) {
				if ap.Name == "noauth" {
					noauthServer = machinery.NewNoauthServer(ctx, ap)
				}
			},
		)
		if !found {
			lg.With(
				"config", configLocation,
			).Error("config file does not contain a GatewayConfig object")
			os.Exit(1)
		}

		lg.With(
			"dir", gatewayConfig.Spec.Plugins.Dir,
		).Info("loading plugins")
		pluginLoader := plugins.NewPluginLoader(plugins.WithLogger(lg.WithGroup("gateway")))

		lifecycler := config.NewLifecycler(objects)
		g := gateway.NewGateway(ctx, gatewayConfig, pluginLoader,
			gateway.WithLifecycler(lifecycler),
			gateway.WithExtraUpdateHandlers(noop.NewSyncServer()),
		)

		m := management.NewServer(ctx, &gatewayConfig.Spec.Management, g, pluginLoader,
			management.WithCapabilitiesDataSource(g.CapabilitiesDataSource()),
			management.WithHealthStatusDataSource(g),
			management.WithLifecycler(lifecycler),
		)

		g.MustRegisterCollector(m)

		doneLoadingPlugins := make(chan struct{})
		pluginLoader.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
			lg.Info(fmt.Sprintf("loaded %d plugins", numLoaded))
			close(doneLoadingPlugins)
		}))
		pluginLoader.LoadPlugins(ctx, gatewayConfig.Spec.Plugins.Dir, plugins.GatewayScheme)
		select {
		case <-doneLoadingPlugins:
		case <-ctx.Done():
			return ctx.Err()
		}

		var eg errgroup.Group
		eg.Go(func() error {
			err := g.ListenAndServe(ctx)
			if errors.Is(err, context.Canceled) {
				lg.Info("gateway server stopped")
			} else if err != nil {
				lg.With(logger.Err(err)).Warn("gateway server exited with error")
			}
			return err
		})
		eg.Go(func() error {
			err := m.ListenAndServe(ctx)
			if errors.Is(err, context.Canceled) {
				lg.Info("management server stopped")
			} else if err != nil {
				lg.With(logger.Err(err)).Warn("management server exited with error")
			}
			return err
		})

		d, err := dashboard.NewServer(&gatewayConfig.Spec.Management)
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to start dashboard server")
		} else {
			eg.Go(func() error {
				err := d.ListenAndServe(ctx)
				if errors.Is(err, context.Canceled) {
					lg.Info("dashboard server stopped")
				} else if err != nil {
					lg.With(logger.Err(err)).Warn("dashboard server exited with error")
				}
				return err
			})
		}

		if noauthServer != nil {
			eg.Go(func() error {
				err := noauthServer.ListenAndServe(ctx)
				if errors.Is(err, context.Canceled) {
					lg.Info("noauth server stopped")
				} else if err != nil {
					lg.With(logger.Err(err)).Warn("noauth server exited with error")
				}
				return err
			})
		}

		errC := lo.Async(eg.Wait)

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
					logger.Err(err),
				).Error("failed to get reload channel from lifecycler")
				os.Exit(1)
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

		shutDownPlugins := func() {
			lg.Info("shutting down plugins")
			plugin.CleanupClients()
			lg.Info("all plugins shut down")
		}
		select {
		case <-reloadC:
			shutDownPlugins()
			atomic.StoreUint32(&plugin.Killed, 0)
			lg.Info(style.Style("--- reloading ---"))
			return nil
		case err := <-errC:
			shutDownPlugins()
			return err
		}
	}

	serveCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the Opni Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			for {
				if err := run(cmd.Context()); err != nil {
					if err == cmd.Context().Err() {
						return nil
					}
					return err
				}
			}
		},
	}

	serveCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return serveCmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildGatewayCmd())
}

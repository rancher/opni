//go:build !minimal && !cli

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	opnicorev1 "github.com/rancher/opni/apis/core/v1"
	"github.com/rancher/opni/pkg/config/reactive"
	"github.com/rancher/opni/pkg/config/reactive/subtle"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/update/noop"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/rancher/opni/pkg/validation"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"

	_ "github.com/rancher/opni/pkg/oci/kubernetes"
	_ "github.com/rancher/opni/pkg/oci/noop"
	_ "github.com/rancher/opni/pkg/plugins/apis"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/storage/crds"
	_ "github.com/rancher/opni/pkg/storage/crds"
	_ "github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/storage/inmemory"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/storage/kvutil"
)

// func BuildGatewayCmd() *cobra.Command {
// 	lg := logger.New()
// 	var configLocation string

// 	run := func(ctx context.Context) error {
// 		tracing.Configure("gateway")

// 		objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)

// 		inCluster := true
// 		restconfig, err := rest.InClusterConfig()
// 		if err != nil {
// 			if errors.Is(err, rest.ErrNotInCluster) {
// 				inCluster = false
// 			} else {
// 				lg.Error(fmt.Sprintf("failed to create config: %s", err))
// 				os.Exit(1)
// 			}
// 		}

// 		if inCluster {
// 			features.PopulateFeatures(ctx, restconfig)
// 			fCancel := features.FeatureList.WatchConfigMap()
// 			context.AfterFunc(ctx, fCancel)
// 		}

// 		machinery.LoadAuthProviders(ctx, objects)
// 		var gatewayConfig *v1beta1.GatewayConfig
// 		var noauthServer *noauth.Server
// 		found := objects.Visit(
// 			func(config *v1beta1.GatewayConfig) {
// 				if gatewayConfig == nil {
// 					gatewayConfig = config
// 				}
// 			},
// 			func(ap *v1beta1.AuthProvider) {
// 				if ap.Name == "noauth" {
// 					noauthServer = machinery.NewNoauthServer(ctx, ap)
// 				}
// 			},
// 		)
// 		if !found {
// 			lg.With(
// 				"config", configLocation,
// 			).Error("config file does not contain a GatewayConfig object")
// 			os.Exit(1)
// 		}

// 		lg.With(
// 			"dir", gatewayConfig.Spec.Plugins.Dir,
// 		).Info("loading plugins")
// 		pluginLoader := plugins.NewPluginLoader(plugins.WithLogger(lg.WithGroup("gateway")))

// 		lifecycler := config.NewLifecycler(objects)
// 		g := gateway.NewGateway(ctx, gatewayConfig, pluginLoader,
// 			gateway.WithLifecycler(lifecycler),
// 			gateway.WithExtraUpdateHandlers(noop.NewSyncServer()),
// 		)

// 		m := management.NewServer(ctx, &gatewayConfig.Spec.Management, g, pluginLoader,
// 			management.WithCapabilitiesDataSource(g.CapabilitiesDataSource()),
// 			management.WithHealthStatusDataSource(g),
// 			management.WithLifecycler(lifecycler),
// 		)

// 		g.MustRegisterCollector(m)

// 		doneLoadingPlugins := make(chan struct{})
// 		pluginLoader.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
// 			lg.Info(fmt.Sprintf("loaded %d plugins", numLoaded))
// 			close(doneLoadingPlugins)
// 		}))
// 		pluginLoader.LoadPlugins(ctx, gatewayConfig.Spec.Plugins.Dir, plugins.GatewayScheme)
// 		select {
// 		case <-doneLoadingPlugins:
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		}

// 		var eg errgroup.Group
// 		eg.Go(func() error {
// 			err := g.ListenAndServe(ctx)
// 			if errors.Is(err, context.Canceled) {
// 				lg.Info("gateway server stopped")
// 			} else if err != nil {
// 				lg.With(logger.Err(err)).Warn("gateway server exited with error")
// 			}
// 			return err
// 		})
// 		eg.Go(func() error {
// 			err := m.ListenAndServe(ctx)
// 			if errors.Is(err, context.Canceled) {
// 				lg.Info("management server stopped")
// 			} else if err != nil {
// 				lg.With(logger.Err(err)).Warn("management server exited with error")
// 			}
// 			return err
// 		})

// 		d, err := dashboard.NewServer(&gatewayConfig.Spec.Management, pluginLoader, g)
// 		if err != nil {
// 			lg.With(logger.Err(err)).Error("failed to start dashboard server")
// 		} else {
// 			eg.Go(func() error {
// 				err := d.ListenAndServe(ctx)
// 				if errors.Is(err, context.Canceled) {
// 					lg.Info("dashboard server stopped")
// 				} else if err != nil {
// 					lg.With(logger.Err(err)).Warn("dashboard server exited with error")
// 				}
// 				return err
// 			})
// 		}

// 		if noauthServer != nil {
// 			eg.Go(func() error {
// 				err := noauthServer.ListenAndServe(ctx)
// 				if errors.Is(err, context.Canceled) {
// 					lg.Info("noauth server stopped")
// 				} else if err != nil {
// 					lg.With(logger.Err(err)).Warn("noauth server exited with error")
// 				}
// 				return err
// 			})
// 		}

// 		errC := lo.Async(eg.Wait)

// 		style := chalk.Yellow.NewStyle().
// 			WithBackground(chalk.ResetColor).
// 			WithTextStyle(chalk.Bold)
// 		reloadC := make(chan struct{})
// 		go func() {
// 			c, err := lifecycler.ReloadC()

// 			fNotify := make(<-chan struct{})
// 			if inCluster {
// 				fNotify = features.FeatureList.NotifyChange()
// 			}

// 			if err != nil {
// 				lg.With(
// 					logger.Err(err),
// 				).Error("failed to get reload channel from lifecycler")
// 				os.Exit(1)
// 			}
// 			select {
// 			case <-c:
// 				lg.Info(style.Style("--- received reload signal ---"))
// 				close(reloadC)
// 			case <-fNotify:
// 				lg.Info(style.Style("--- received feature update signal ---"))
// 				close(reloadC)
// 			}
// 		}()

// 		shutDownPlugins := func() {
// 			lg.Info("shutting down plugins")
// 			plugin.CleanupClients()
// 			lg.Info("all plugins shut down")
// 		}
// 		select {
// 		case <-reloadC:
// 			shutDownPlugins()
// 			atomic.StoreUint32(&plugin.Killed, 0)
// 			lg.Info(style.Style("--- reloading ---"))
// 			return nil
// 		case err := <-errC:
// 			shutDownPlugins()
// 			return err
// 		}
// 	}

// 	serveCmd := &cobra.Command{
// 		Use:   "gateway",
// 		Short: "Run the Opni Gateway",
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			for {
// 				if err := run(cmd.Context()); err != nil {
// 					if err == cmd.Context().Err() {
// 						return nil
// 					}
// 					return err
// 				}
// 			}
// 		},
// 	}

// 	serveCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
// 	return serveCmd
// }

func BuildGatewayCmd() *cobra.Command {
	config := &configv1.GatewayConfigSpec{}
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the Opni Gateway",
		Long: `
Flags available for this command are defaults, generally only used for the
initial setup of the Opni Gateway. Most users will want to configure
the gateway after installation using the dashboard UI or via the 'opni config'
CLI command.

The values of these flags are defaults that are used as an initial base
configuration, and can be reverted to if necessary.

When running in Kubernetes, the Gateway custom resource is the corresponding
"active" configuration. Using the dashboard or CLI to change these settings
will update the custom resource; if possible, avoid editing it directly.

Outside of Kubernetes, the active configuration is persisted in the KV store.
Regardless of runtime environment, the config APIs work the same way.

These defaults can be modified at runtime, but changes to the defaults will not
be persisted across restarts. To persist changes, use the active configuration
mechanisms described above. Note that similar configuration APIs for other
Opni components generally do persist their default config in the KV store. The
Gateway is different in this regard.
`[1:],
		RunE: func(cmd *cobra.Command, args []string) error {
			lg := logger.New()
			ctx := cmd.Context()

			tracing.Configure("gateway")
			v, err := validation.NewValidator()
			if err != nil {
				return err
			}

			var storageBackend storage.Backend

			defaultStore := inmemory.NewValueStore[*configv1.GatewayConfigSpec](util.ProtoClone)
			if err := v.Validate(config); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				return errors.New("exiting due to validation errors")
			}
			if err := defaultStore.Put(ctx, config); err != nil {
				return fmt.Errorf("failed to set defaults from flags: %w", err)
			}

			var activeStore storage.ValueStoreT[*configv1.GatewayConfigSpec]

			if inCluster() {
				lg.Info("loading config (in-cluster)")
				activeStore = crds.NewCRDValueStore[*opnicorev1.Gateway, *configv1.GatewayConfigSpec](types.NamespacedName{
					Namespace: os.Getenv("POD_NAMESPACE"),
					Name:      os.Getenv("GATEWAY_NAME"),
				}, opnicorev1.ValueStoreMethods{})
				active, err := activeStore.Get(ctx)
				if err != nil {
					return err
				}
				storageBackend, err = machinery.ConfigureStorageBackendV1(ctx, active.GetStorage())
				if err != nil {
					return err
				}
				activeStore = kvutil.WithMessageCodec[*configv1.GatewayConfigSpec](
					kvutil.WithKey(storageBackend.KeyValueStore("gateway"), "config"))
			} else {
				lg.Info("loading config")
				storageBackend, err = machinery.ConfigureStorageBackendV1(ctx, config.GetStorage())
				if err != nil {
					return err
				}
				lg.Info("storage configured", "backend", config.Storage.GetBackend().String())

				activeStore = kvutil.WithMessageCodec[*configv1.GatewayConfigSpec](
					kvutil.WithKey(storageBackend.KeyValueStore("gateway"), "config"))
			}
			mgr := configv1.NewGatewayConfigManager(defaultStore, activeStore, func(gcs *configv1.GatewayConfigSpec) {
				flagutil.LoadDefaults(gcs)
				merge.MergeWithReplace(gcs, config)
			}, configv1.WithControllerOptions(
				reactive.WithLogger(lg.WithGroup("config")),
				reactive.WithDiffMode(reactive.DiffFull),
			))

			if _, err := mgr.Tracker().ActiveStore().Get(context.Background()); err != nil {
				if storage.IsNotFound(err) {
					lg.Info("no previous configuration found, creating from defaults")
					_, err := mgr.SetConfiguration(context.Background(), &configv1.SetRequest{})
					if err != nil {
						return fmt.Errorf("failed to set configuration: %w", err)
					}
				}
			} else {
				lg.Info("loaded existing configuration")
			}

			if err := mgr.Start(ctx); err != nil {
				return fmt.Errorf("failed to start config manager: %w", err)
			}

			pluginLoader := plugins.NewPluginLoader(plugins.WithLogger(lg.WithGroup("gateway")))

			g := gateway.NewGateway(ctx, mgr, storageBackend, pluginLoader,
				gateway.WithExtraUpdateHandlers(noop.NewSyncServer()),
			)

			m := management.NewServer(ctx, g, mgr, pluginLoader,
				management.WithCapabilitiesDataSource(g.CapabilitiesDataSource()),
				management.WithHealthStatusDataSource(g),
			)

			g.MustRegisterCollector(m)

			doneLoadingPlugins := make(chan struct{})
			dir := subtle.WaitOne(ctx, mgr.Reactive(config.ProtoPath().Plugins().Dir())).String()
			pluginLoader.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
				lg.Info(fmt.Sprintf("loaded %d plugins", numLoaded))
				close(doneLoadingPlugins)
			}))
			pluginLoader.LoadPlugins(ctx, dir, plugins.GatewayScheme)
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

			return eg.Wait()
		},
	}
	cmd.Flags().AddFlagSet(config.FlagSet("defaults"))
	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildGatewayCmd())
}

func inCluster() bool {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return false
	}
	return true
}

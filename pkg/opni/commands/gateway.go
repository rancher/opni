//go:build !minimal && !cli

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/bufbuild/protovalidate-go"
	"github.com/nsf/jsondiff"
	opnicorev1 "github.com/rancher/opni/apis/core/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
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
	"github.com/rancher/opni/pkg/util/fieldmask"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/validation"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"

	_ "github.com/rancher/opni/pkg/oci/kubernetes"
	_ "github.com/rancher/opni/pkg/oci/noop"
	_ "github.com/rancher/opni/pkg/plugins/apis"
	"github.com/rancher/opni/pkg/plugins/driverutil"
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
	var inCluster bool
	host, hostOk := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	port, portOk := os.LookupEnv("KUBERNETES_SERVICE_PORT")
	if hostOk && portOk && host != "" && port != "" {
		inCluster = true
	}

	var applyDefaultFlags bool
	var ignoreValidationErrors bool
	storageConfig := &configv1.StorageSpec{}
	config := &configv1.GatewayConfigSpec{}
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the Opni Gateway",
		Long: `
Flags and Configuration
===========================
Flags for this command are split into two categories: 'storage' flags and
'default' flags. Though the storage spec is contained within the complete
gateway configuration, but is separated for startup and initialization purposes,


Storage flags (--storage.*)
===========================
These flags configure the storage backend used to persist and retrieve
the active configuration. The values set by these flags will take precedence
over the corresponding values in the active configuration, if it exists.
* If running in Kubernetes, these flags are ignored.


Default flags (--default.*)
===========================
These flags allow adjusting the default values used when starting up the
gateway for the first time. They are all optional, and are ignored once
an active configuration has been created. However, if --apply-default-flags
is set, if there is an existing active configuration, any --default.* flags
listed on the command line will be applied to the active configuration before
starting the gateway.

* If running in Kubernetes, these flags are ignored, and --apply-default-flags
  has no effect.


Startup Logic
===========================
If the gateway is running inside a Kubernetes cluster, see the section below
for startup logic specific to Kubernetes. Regardless of runtime environment,
the runtime config APIs work the same way.

The gateway startup logic is as follows:

When the gateway starts up, it uses its storage flags (--storage.*) to connect
to a KV store and look for an active configuration.
- If there is no existing active config in the KV store, it will create one
  with the default values from its flags (--default.*).
- If there is an existing active config, it will use that. Additionally, before
  starting the gateway, if --apply-default-flags is set, it will apply any
  --default.* flags listed on the command line to the active configuration,
  overwriting any existing values, and persisting the changes to the KV store.


Startup Logic (Kubernetes)
===========================
When the gateway is running inside Kubernetes, it will look for the Gateway
custom resource that controls the running pod. Because this custom resource
controls the deployment of the gateway pod itself, it assumes it will always
exist. If it does not exist, the gateway will exit with an error.

When running in Kubernetes, the 'config' field in the Gateway custom resource
is the corresponding "active" configuration. Using the dashboard or CLI to
change these settings will update the custom resource; if possible, avoid
editing it directly.

In Kubernetes, all flags are ignored. Because the active configuration is
always present in the Gateway custom resource, it does not need to supply
its own default values or storage settings.


Runtime Config APIs
===========================
Once the gateway has started, the active configuration can be modified at
runtime using the dashboard UI or the 'opni config' CLI command.

Changes to the active configuration will be persisted to the KV store, and
relevant components will be notified of changed fields and will reload their
configuration accordingly (restarting servers, etc).

Changes to the default configuration, while possible, will not be persisted
across restarts. The gateway only stores its default configuration in-memory.
Note that similar configuration APIs for other Opni components generally do
persist their default configurations in the KV store.
`[1:],
		RunE: func(cmd *cobra.Command, args []string) error {
			lg := logger.New()
			ctx := cmd.Context()
			tracing.Configure("gateway")

			var storageBackend storage.Backend

			defaultStore := inmemory.NewValueStore[*configv1.GatewayConfigSpec](util.ProtoClone)

			if !inCluster {
				v, err := validation.NewValidator()
				if err != nil {
					return err
				}
				if err := v.Validate(config); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
					return errors.New("exiting due to validation errors")
				}
				if err := defaultStore.Put(ctx, config); err != nil {
					return fmt.Errorf("failed to set defaults from flags: %w", err)
				}
			}
			var activeStore storage.ValueStoreT[*configv1.GatewayConfigSpec]

			if inCluster {
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
			} else {
				lg.Info("loading config")
				var err error
				storageBackend, err = machinery.ConfigureStorageBackendV1(ctx, config.GetStorage())
				if err != nil {
					return err
				}
				lg.Info("storage configured", "backend", config.Storage.GetBackend().String())
			}
			activeStore = kvutil.WithMessageCodec[*configv1.GatewayConfigSpec](
				kvutil.WithKey(storageBackend.KeyValueStore("gateway"), "config"))

			mgr := configv1.NewGatewayConfigManager(
				defaultStore, activeStore,
				flagutil.LoadDefaults,
				configv1.WithControllerOptions(
					reactive.WithLogger(lg.WithGroup("config")),
					reactive.WithDiffMode(reactive.DiffFull),
				),
			)

			if !inCluster {
				var rev *corev1.Revision
				if ac, err := mgr.Tracker().ActiveStore().Get(context.Background()); err != nil {
					if storage.IsNotFound(err) {
						lg.Info("no previous configuration found, creating from defaults")
						_, err := mgr.SetConfiguration(context.Background(), &configv1.SetRequest{})
						if err != nil {
							return fmt.Errorf("failed to set configuration: %w", err)
						}
					}
				} else {
					rev = ac.GetRevision()
					lg.Info("loaded existing configuration", "rev", ac.GetRevision().GetRevision())
				}

				if applyDefaultFlags {
					mask := fieldmask.ByPresence(config.ProtoReflect())
					resp, err := mgr.DryRun(ctx, &configv1.DryRunRequest{
						Target:   driverutil.Target_ActiveConfiguration,
						Action:   driverutil.Action_Reset,
						Revision: rev,
						Patch:    config,
						Mask:     mask,
					})
					if err != nil {
						return err
					}
					opts := jsondiff.DefaultConsoleOptions()
					opts.SkipMatches = true
					diff, anyChanges := driverutil.RenderJsonDiff(resp.Current, resp.Modified, opts)
					stat := driverutil.DiffStat(diff, opts)
					if anyChanges {
						lg.Info("applying default flags to active configuration", "diff", stat)
						lg.Info("⤷ diff:\n" + diff)
					} else {
						lg.Warn("--apply-default-flags was set, but no changes would be made to the active configuration")
					}
					if resp.GetValidationErrors() != nil {
						lg.Error("refusing to apply default flags due to validation errors (re-run with --ignore-validation-errors to skip this check)")
						return (*protovalidate.ValidationError)(resp.ValidationErrors)
					}
					if anyChanges {
						_, err := mgr.ResetConfiguration(ctx, &configv1.ResetRequest{
							Revision: rev,
							Mask:     mask,
							Patch:    config,
						})
						if err != nil {
							return err
						}
					}
				}
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
	if !inCluster {
		cmd.Flags().AddFlagSet(storageConfig.FlagSet("storage"))
		cmd.Flags().AddFlagSet(config.FlagSet("defaults"))
		cmd.Flags().BoolVar(&applyDefaultFlags, "apply-default-flags", false,
			"Apply default flags listed on the command-line to the active configuration on startup")
		cmd.Flags().BoolVar(&ignoreValidationErrors, "ignore-validation-errors", false, "Ignore validation errors when applying default flags")
		cmd.Flags().MarkHidden("ignore-validation-errors")
	}
	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildGatewayCmd())
}

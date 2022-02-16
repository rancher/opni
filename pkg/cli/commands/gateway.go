package commands

import (
	"context"
	"errors"
	"os"
	"sync/atomic"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/noauth"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/gateway"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
)

func BuildGatewayCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	run := func() error {
		if configLocation == "" {
			// find config file
			path, err := config.FindConfig()
			if err != nil {
				if errors.Is(err, config.ErrConfigNotFound) {
					wd, _ := os.Getwd()
					lg.Fatalf(`could not find a config file in ["%s","/etc/opni-monitoring"], and --config was not given`, wd)
				}
				lg.With(
					zap.Error(err),
				).Fatal("an error occurred while searching for a config file")
			}
			lg.With(
				"path", path,
			).Info("using config file")
			configLocation = path
		}

		ctx, cancel := context.WithCancel(waitctx.FromContext(context.Background()))
		objects, err := config.LoadObjectsFromFile(configLocation)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Fatal("failed to load config")
		}
		var gatewayConfig *v1beta1.GatewayConfig
		objects.Visit(
			func(config *v1beta1.GatewayConfig) {
				if gatewayConfig == nil {
					gatewayConfig = config
				}
			},
			func(ap *v1beta1.AuthProvider) {
				switch ap.Spec.Type {
				case v1beta1.AuthProviderOpenID:
					mw, err := openid.New(ctx, ap.Spec)
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to create OpenID auth provider")
					}
					if err := auth.RegisterMiddleware(ap.GetName(), mw); err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to register OpenID auth provider")
					}
				case v1beta1.AuthProviderNoAuth:
					mw, err := noauth.New(ctx, ap.Spec)
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to create noauth auth provider")
					}
					if err := auth.RegisterMiddleware(ap.GetName(), mw); err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to register noauth auth provider")
					}
				default:
					lg.With(
						"type", ap.Spec.Type,
					).Fatal("unsupported auth provider type")
				}
			},
		)

		lifecycler := config.NewLifecycler(objects)
		g := gateway.NewGateway(ctx, gatewayConfig,
			gateway.WithLifecycler(lifecycler),
			gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
		)

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

		if err := g.Listen(); err != nil {
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

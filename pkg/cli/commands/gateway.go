package commands

import (
	"errors"
	"os"

	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/noauth"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/gateway"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/spf13/cobra"
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
					mw, err := openid.New(ap.Spec)
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
					mw, err := noauth.New(ap.Spec)
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

		g := gateway.NewGateway(gatewayConfig,
			gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
			gateway.WithPrefork(false),
		)

		return g.Listen()
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

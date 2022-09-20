package commands

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	agentv2 "github.com/rancher/opni/pkg/agent/v2"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
)

func BuildAgentV2Cmd() *cobra.Command {
	var configFile, logLevel string
	cmd := &cobra.Command{
		Use:   "agentv2",
		Short: "Run the v2 agent",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := waitctx.FromContext(cmd.Context())

			tracing.Configure("agentv2")
			agentlg = logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(logLevel))))

			if configFile == "" {
				// find config file
				path, err := config.FindConfig()
				if err != nil {
					if errors.Is(err, config.ErrConfigNotFound) {
						wd, _ := os.Getwd()
						agentlg.Fatalf(`could not find a config file in ["%s","/etc/opni-monitoring"], and --config was not given`, wd)
					}
					agentlg.With(
						zap.Error(err),
					).Fatal("an error occurred while searching for a config file")
				}
				agentlg.With(
					"path", path,
				).Info("using config file")
				configFile = path
			}

			objects, err := config.LoadObjectsFromFile(configFile)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to load config")
			}
			var agentConfig *v1beta1.AgentConfig
			if ok := objects.Visit(func(config *v1beta1.AgentConfig) {
				agentConfig = config
			}); !ok {
				agentlg.Fatal("no agent config found in config file")
			}

			if agentConfig.Spec.Profiling {
				fmt.Fprintln(os.Stderr, chalk.Yellow.Color("Profiling is enabled. This should only be used for debugging purposes."))
				runtime.SetBlockProfileRate(10000)
				runtime.SetMutexProfileFraction(100)
			}

			bootstrapper, err := configureBootstrapV2(agentConfig)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to configure bootstrap")
			}

			//pl := plugins.NewPluginLoader()

			p, err := agentv2.New(ctx, agentConfig,
				agentv2.WithBootstrapper(bootstrapper),
			)
			if err != nil {
				agentlg.Error(err)
				return
			}

			err = p.ListenAndServe(ctx)
			if err != nil {
				agentlg.Error(err)
				return
			}

			//pl.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
			//	lg.Infof("loaded %d plugins", numLoaded)
			//}))
			//
			//pl.Hook(hooks.OnLoadingCompleted(func(int) {
			//	waitctx.AddOne(ctx)
			//	defer waitctx.Done(ctx)
			//	if err := p.ListenAndServe(ctx); err != nil {
			//		lg.With(
			//			zap.Error(err),
			//		).Warn("agent server exited with error")
			//	}
			//}))
			//
			//pl.LoadPlugins(ctx, agentConfig.Spec.Plugins, plugins.AgentScheme)

			<-ctx.Done()
			waitctx.Wait(ctx)
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "Absolute path to a config file")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warning, error)")
	return cmd
}

func configureBootstrapV2(conf *v1beta1.AgentConfig) (bootstrap.Bootstrapper, error) {
	var bootstrapper bootstrap.Bootstrapper
	var trustStrategy trust.Strategy
	if conf.Spec.Bootstrap == nil {
		return nil, errors.New("no bootstrap config provided")
	}
	if conf.Spec.Bootstrap.InClusterManagementAddress != nil {
		bootstrapper = &bootstrap.InClusterBootstrapperV2{
			GatewayEndpoint:    conf.Spec.GatewayAddress,
			ManagementEndpoint: *conf.Spec.Bootstrap.InClusterManagementAddress,
		}
	} else {
		agentlg.Info("loading bootstrap tokens from config file")
		tokenData := conf.Spec.Bootstrap.Token

		switch conf.Spec.TrustStrategy {
		case v1beta1.TrustStrategyPKP:
			var err error
			pins := conf.Spec.Bootstrap.Pins
			publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
			for i, pin := range pins {
				publicKeyPins[i], err = pkp.DecodePin(pin)
				if err != nil {
					agentlg.With(
						zap.Error(err),
						zap.String("pin", string(pin)),
					).Error("failed to parse pin")
					return nil, err
				}
			}
			conf := trust.StrategyConfig{
				PKP: &trust.PKPConfig{
					Pins: trust.NewPinSource(publicKeyPins),
				},
			}
			trustStrategy, err = conf.Build()
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Error("error configuring PKP trust strategy")
				return nil, err
			}
		case v1beta1.TrustStrategyCACerts:
			paths := conf.Spec.Bootstrap.CACerts
			certs := []*x509.Certificate{}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					agentlg.With(
						zap.Error(err),
						zap.String("path", path),
					).Error("failed to read CA cert")
					return nil, err
				}
				cert, err := util.ParsePEMEncodedCert(data)
				if err != nil {
					agentlg.With(
						zap.Error(err),
						zap.String("path", path),
					).Error("failed to parse CA cert")
					return nil, err
				}
				certs = append(certs, cert)
			}
			conf := trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: trust.NewCACertsSource(certs),
				},
			}
			var err error
			trustStrategy, err = conf.Build()
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Error("error configuring CA Certs trust strategy")
				return nil, err
			}
		case v1beta1.TrustStrategyInsecure:
			agentlg.Warn(chalk.Bold.NewStyle().WithForeground(chalk.Yellow).Style(
				"*** Using insecure trust strategy. This is not recommended. ***",
			))
			conf := trust.StrategyConfig{
				Insecure: &trust.InsecureConfig{},
			}
			var err error
			trustStrategy, err = conf.Build()
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Error("error configuring insecure trust strategy")
				return nil, err
			}
		}

		token, err := tokens.ParseHex(tokenData)
		if err != nil {
			agentlg.With(
				zap.Error(err),
				zap.String("token", fmt.Sprintf("[redacted (len: %d)]", len(tokenData))),
			).Error("failed to parse token")
			return nil, err
		}
		bootstrapper = &bootstrap.ClientConfigV2{
			Token:         token,
			Endpoint:      conf.Spec.GatewayAddress,
			TrustStrategy: trustStrategy,
		}
	}

	return bootstrapper, nil
}

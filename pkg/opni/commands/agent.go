package commands

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
)

func BuildAgentCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the Opni Monitoring Agent",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		Run: func(cmd *cobra.Command, args []string) {
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
			var agentConfig *v1beta1.AgentConfig
			objects.Visit(func(config *v1beta1.AgentConfig) {
				agentConfig = config
			})

			var bootstrapper bootstrap.Bootstrapper
			var trustStrategy trust.Strategy
			if agentConfig.Spec.Bootstrap != nil {
				lg.Info("loading bootstrap tokens from config file")
				tokenData := agentConfig.Spec.Bootstrap.Token

				switch agentConfig.Spec.TrustStrategy {
				case v1beta1.TrustStrategyPKP:
					pins := agentConfig.Spec.Bootstrap.Pins
					publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
					for i, pin := range pins {
						publicKeyPins[i], err = pkp.DecodePin(pin)
						if err != nil {
							lg.With(
								zap.Error(err),
								zap.String("pin", string(pin)),
							).Error("failed to parse pin")
						}
					}
					conf := trust.StrategyConfig{
						PKP: &trust.PKPConfig{
							Pins: trust.NewPinSource(publicKeyPins),
						},
					}
					trustStrategy, err = conf.Build()
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("error configuring PKP trust strategy")
					}
				case v1beta1.TrustStrategyCACerts:
					paths := agentConfig.Spec.Bootstrap.CACerts
					certs := []*x509.Certificate{}
					for _, path := range paths {
						data, err := os.ReadFile(path)
						if err != nil {
							lg.With(
								zap.Error(err),
								zap.String("path", path),
							).Fatal("failed to read CA cert")
						}
						cert, err := util.ParsePEMEncodedCert(data)
						if err != nil {
							lg.With(
								zap.Error(err),
								zap.String("path", path),
							).Fatal("failed to parse CA cert")
						}
						certs = append(certs, cert)
					}
					conf := trust.StrategyConfig{
						CACerts: &trust.CACertsConfig{
							CACerts: trust.NewCACertsSource(certs),
						},
					}
					trustStrategy, err = conf.Build()
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("error configuring CA Certs trust strategy")
					}
				case v1beta1.TrustStrategyInsecure:
					lg.Warn(chalk.Bold.NewStyle().WithForeground(chalk.Yellow).Style(
						"*** Using insecure trust strategy. This is not recommended. ***",
					))
					conf := trust.StrategyConfig{
						Insecure: &trust.InsecureConfig{},
					}
					trustStrategy, err = conf.Build()
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("error configuring insecure trust strategy")
					}
				}

				token, err := tokens.ParseHex(tokenData)
				if err != nil {
					lg.With(
						zap.Error(err),
						zap.String("token", fmt.Sprintf("[redacted (len: %d)]", len(tokenData))),
					).Error("failed to parse token")
				}
				bootstrapper = &bootstrap.ClientConfig{
					Capability:    wellknown.CapabilityMetrics,
					Token:         token,
					Endpoint:      agentConfig.Spec.GatewayAddress,
					TrustStrategy: trustStrategy,
				}
			}

			p, err := agent.New(cmd.Context(), agentConfig,
				agent.WithBootstrapper(bootstrapper),
			)
			if err != nil {
				lg.Error(err)
				return
			}
			if err := p.ListenAndServe(); err != nil {
				lg.Error(err)
			}
		},
	}

	agentCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return agentCmd
}

package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/kralicky/opni-monitoring/pkg/agent"
	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/config"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	"github.com/spf13/cobra"
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
			if agentConfig.Spec.Bootstrap != nil {
				lg.Info("loading bootstrap tokens from config file")
				tokenData := agentConfig.Spec.Bootstrap.Token
				pins := agentConfig.Spec.Bootstrap.Pins
				token, err := tokens.ParseHex(tokenData)
				if err != nil {
					lg.With(
						zap.Error(err),
						zap.String("token", fmt.Sprintf("[redacted (len: %d)]", len(tokenData))),
					).Error("failed to parse token")
				}
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
				bootstrapper = &bootstrap.ClientConfig{
					Token:    token,
					Pins:     publicKeyPins,
					Endpoint: agentConfig.Spec.GatewayAddress,
				}
			}

			p := agent.New(agentConfig, agent.WithBootstrapper(bootstrapper))
			if err := p.ListenAndServe(); err != nil {
				lg.Error(err)
			}
		},
	}

	agentCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return agentCmd
}

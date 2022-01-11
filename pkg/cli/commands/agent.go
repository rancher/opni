package commands

import (
	"errors"
	"log"
	"os"

	"github.com/kralicky/opni-monitoring/pkg/agent"
	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/config"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	"github.com/spf13/cobra"
)

func BuildAgentCmd() *cobra.Command {
	var configLocation string

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the Opni Monitoring Agent",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configLocation == "" {
				// find config file
				path, err := config.FindConfig()
				if err != nil {
					if errors.Is(err, config.ErrConfigNotFound) {
						wd, _ := os.Getwd()
						log.Fatalf(`could not find a config file in ["%s","/etc/opni-monitoring"], and --config was not given`, wd)
					}
					log.Fatalf("an error occurred while searching for a config file: %v", err)
				}
				log.Println("using config file:", path)
				configLocation = path
			}

			objects, err := config.LoadObjectsFromFile(configLocation)
			if err != nil {
				log.Fatalf("failed to load config: %v", err)
			}
			var agentConfig *v1beta1.AgentConfig
			objects.Visit(func(config *v1beta1.AgentConfig) {
				agentConfig = config
			})

			tokenData := agentConfig.Spec.Bootstrap.Token
			pins := agentConfig.Spec.Bootstrap.Pins
			token, err := tokens.ParseHex(tokenData)
			if err != nil {
				return err
			}
			publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
			for i, pin := range pins {
				publicKeyPins[i], err = pkp.DecodePin(pin)
			}

			p := agent.New(agentConfig,
				agent.WithBootstrapper(&bootstrap.ClientConfig{
					Token:    token,
					Pins:     publicKeyPins,
					Endpoint: agentConfig.Spec.GatewayAddress,
				}),
			)
			return p.ListenAndServe()
		},
	}

	agentCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return agentCmd
}

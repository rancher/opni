package commands

import (
	"encoding/hex"
	"errors"
	"log"
	"os"

	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/proxy"
	"github.com/kralicky/opni-gateway/pkg/tokens"
	"github.com/spf13/cobra"
)

func BuildProxyCmd() *cobra.Command {
	var configLocation string
	var hexToken, caCertHash string

	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "Run the gateway proxy",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configLocation == "" {
				// find config file
				path, err := config.FindConfig()
				if err != nil {
					if errors.Is(err, config.ErrConfigNotFound) {
						wd, _ := os.Getwd()
						log.Fatalf(`could not find a config file in ["%s","/etc/opni-gateway"], and --config was not given`, wd)
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
			var proxyConfig *v1beta1.ProxyConfig
			objects.Visit(func(config *v1beta1.ProxyConfig) {
				proxyConfig = config
			})

			token, err := tokens.DecodeHexToken(hexToken)
			if err != nil {
				return err
			}
			caCertHashData, err := hex.DecodeString(caCertHash)
			if err != nil {
				return err
			}
			p := proxy.NewRemoteWriteProxy(proxyConfig,
				proxy.WithBootstrapper(&bootstrap.ClientConfig{
					Token:      token,
					CACertHash: caCertHashData,
					Endpoint:   proxyConfig.Spec.GatewayAddress,
				}),
			)
			return p.ListenAndServe()
		},
	}

	proxyCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	proxyCmd.Flags().StringVar(&hexToken, "token", "", "Bootstrap token (hex encoded)")
	proxyCmd.Flags().StringVar(&caCertHash, "ca-cert-hash", "", "CA cert hash (hex encoded)")

	proxyCmd.MarkFlagRequired("token")
	proxyCmd.MarkFlagRequired("ca-cert-hash")
	return proxyCmd
}

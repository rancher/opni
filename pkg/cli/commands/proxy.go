package commands

import (
	"encoding/hex"

	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/ident"
	"github.com/kralicky/opni-gateway/pkg/proxy"
	"github.com/kralicky/opni-gateway/pkg/tokens"
	"github.com/spf13/cobra"
)

func BuildProxyCmd() *cobra.Command {
	var listenAddr, gatewayAddr string
	var hexToken, caCertHash string

	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "Run the gateway proxy",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := tokens.DecodeHexToken(hexToken)
			if err != nil {
				return err
			}
			caCertHashData, err := hex.DecodeString(caCertHash)
			if err != nil {
				return err
			}
			p := proxy.NewRemoteWriteProxy(
				proxy.WithGatewayAddr(gatewayAddr),
				proxy.WithListenAddr(listenAddr),
				proxy.WithBootstrapper(&bootstrap.ClientConfig{
					Token:      token,
					CACertHash: caCertHashData,
					Endpoint:   gatewayAddr,
					Ident:      ident.NewUUIDIdentProvider(),
				}),
			)
			return p.ListenAndServe()
		},
	}

	proxyCmd.Flags().StringVar(&hexToken, "token", "", "Bootstrap token (hex encoded)")
	proxyCmd.Flags().StringVar(&caCertHash, "ca-cert-hash", "", "CA cert hash (hex encoded)")
	proxyCmd.Flags().StringVar(&listenAddr, "listen-addr", ":8080", "Address to listen on")
	proxyCmd.Flags().StringVar(&gatewayAddr, "gateway-addr", "", "Address of the gateway server")

	proxyCmd.MarkFlagRequired("token")
	proxyCmd.MarkFlagRequired("ca-cert-hash")
	proxyCmd.MarkFlagRequired("gateway-addr")
	return proxyCmd
}

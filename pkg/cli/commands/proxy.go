package commands

import (
	"github.com/kralicky/opni-gateway/pkg/proxy"
	"github.com/spf13/cobra"
)

func BuildProxyCmd() *cobra.Command {
	var listenAddr, gatewayAddr string

	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "Run the gateway proxy",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := proxy.NewRemoteWriteProxy(
				proxy.WithGatewayAddr(gatewayAddr),
				proxy.WithListenAddr(listenAddr),
			)
			return p.ListenAndServe()
		},
	}

	proxyCmd.Flags().StringVar(&listenAddr, "listen-addr", ":8080", "Address to listen on")
	proxyCmd.Flags().StringVar(&gatewayAddr, "gateway-addr", "", "Address of the gateway server")
	return proxyCmd
}

package commands

import (
	"context"
	"fmt"

	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/spf13/cobra"
)

func BuildToolCmd() *cobra.Command {
	toolCmd := &cobra.Command{
		Use:   "tool",
		Short: "Tools for working with opni-gateway",
	}
	toolCmd.AddCommand(BuildBootstrapCmd())
	return toolCmd
}

func BuildBootstrapCmd() *cobra.Command {
	var bootstrapToken, cacertHash string
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap server:port",
		Short: "Bootstrap against the opni gateway and print debugging information",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, _, err := bootstrap.ClientConfig{
				Token:      bootstrapToken,
				CACertHash: cacertHash,
				Endpoint:   args[0],
				Ident:      bootstrap.NewUUIDIdentProvider(),
			}.Run()
			if err != nil {
				return err
			}
			fmt.Println("Success")
			fmt.Println(token.AsMap(context.Background()))
			fmt.Printf("Client ID: %s\n", token.Subject())
			return nil
		},
	}
	bootstrapCmd.Flags().StringVar(&bootstrapToken, "token", "", "Secret token to use for bootstrapping (not base64 encoded)")
	bootstrapCmd.Flags().StringVar(&cacertHash, "ca-cert-hash", "", "Hex-encoded SHA256 hash of the root CA certificate")
	return bootstrapCmd
}

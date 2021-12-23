package commands

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/kralicky/opni-gateway/pkg/util"
	"github.com/spf13/cobra"
)

func BuildToolCmd() *cobra.Command {
	toolCmd := &cobra.Command{
		Use:   "tool",
		Short: "Tools for working with opni-gateway",
	}
	toolCmd.AddCommand(BuildCertHashCmd())
	return toolCmd
}

func BuildCertHashCmd() *cobra.Command {
	certHashCmd := &cobra.Command{
		Use:   "cert-hash <filepath>",
		Short: "Generate a CA cert hash used for bootstrapping",
		Long: `Certificates must be in PEM format. If the file contains a certificate chain,
the last certificate will be used.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var block *pem.Block
			rest := data
			for {
				b, r := pem.Decode(rest)
				if b == nil {
					break
				}
				block = b
				rest = r
			}
			if block == nil {
				return errors.New("No PEM blocks found in file")
			}
			if block.Type != "CERTIFICATE" {
				return errors.New("Unexpected block type " + block.Type)
			}
			certificate, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return fmt.Errorf("Failed to parse certificate: %w", err)
			}
			if !certificate.IsCA {
				return errors.New("No CA certificate found")
			}
			fmt.Println(hex.EncodeToString(util.CertSPKIHash(certificate)))
			return nil
		},
	}
	return certHashCmd
}

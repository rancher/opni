package trust

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/pflag"
)

func BindFlags(flags *pflag.FlagSet) {
	flags.String("trust-strategy", string(v1beta1.TrustStrategyPKP),
		"Trust strategy to use for verifying the authenticity of the gateway server. "+
			"Defaults to \"pkp\". Valid values: [\"pkp\", \"cacerts\", \"insecure\"]")
	flags.StringSlice("pin", []string{}, "Gateway server public key to pin (repeatable). "+
		"Used when the trust strategy is \"pkp\"")
	flags.StringSlice("cacert", []string{}, "Path to a PEM-encoded CA Cert (repeatable). "+
		"Used when the trust strategy is \"cacerts\". If no certs are provided, the system certs will be used.")
	flags.Bool("confirm-insecure", false, "Required when the trust strategy is \"insecure\"")
	flags.MarkHidden("confirm-insecure")
}

func BuildConfigFromFlags(flags *pflag.FlagSet) (*StrategyConfig, error) {
	strategy := flags.Lookup("trust-strategy").Value.String()
	switch strategy {
	case string(v1beta1.TrustStrategyPKP):
		pins := flags.Lookup("pin").Value.(pflag.SliceValue).GetSlice()
		if len(pins) == 0 {
			return nil, fmt.Errorf("no pins provided")
		}
		pinnedKeys := []*pkp.PublicKeyPin{}
		for _, pin := range pins {
			pinnedKey, err := pkp.DecodePin(pin)
			if err != nil {
				return nil, err
			}
			pinnedKeys = append(pinnedKeys, pinnedKey)
		}
		return &StrategyConfig{
			PKP: &PKPConfig{
				Pins: NewPinSource(pinnedKeys),
			},
		}, nil
	case string(v1beta1.TrustStrategyCACerts):
		cacerts := flags.Lookup("cacert").Value.(pflag.SliceValue).GetSlice()
		certs := []*x509.Certificate{}
		for _, cacert := range cacerts {
			data, err := os.ReadFile(cacert)
			if err != nil {
				return nil, err
			}
			certchain, err := util.ParsePEMEncodedCertChain(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CA cert: %w", err)
			}
			certs = append(certs, certchain...)
		}
		return &StrategyConfig{
			CACerts: &CACertsConfig{
				CACerts: NewCACertsSource(certs),
			},
		}, nil
	case string(v1beta1.TrustStrategyInsecure):
		if !flags.Lookup("confirm-insecure").Changed {
			warning :=
				"use of the \"insecure\" trust strategy is highly discouraged, except for testing " +
					"purposes, or in an air-gapped environment. If you know the security risks " +
					"associated with implicitly trusting the remote gateway server, or are in an " +
					"environment where such risks do not apply, please re-run the command with the " +
					"\"--confirm-insecure\" flag"
			return nil, errors.New(warning)
		}
		return &StrategyConfig{
			Insecure: &InsecureConfig{},
		}, nil
	default:
		return nil, fmt.Errorf("invalid trust strategy: %s", strategy)
	}
}

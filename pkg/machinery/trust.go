package machinery

import (
	"fmt"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/trust"
)

func BuildTrustStrategy(kind v1beta1.TrustStrategyKind, kr keyring.Keyring) (trust.Strategy, error) {
	var trustStrategy trust.Strategy
	var err error
	switch kind {
	case v1beta1.TrustStrategyPKP:
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: trust.NewKeyringPinSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring pkp trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyCACerts:
		conf := trust.StrategyConfig{
			CACerts: &trust.CACertsConfig{
				CACerts: trust.NewKeyringCACertsSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring ca certs trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyInsecure:
		conf := trust.StrategyConfig{
			Insecure: &trust.InsecureConfig{},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring insecure trust: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown trust strategy: %s", kind)
	}

	return trustStrategy, nil
}

package trust

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/pkp"
)

type Strategy interface {
	TLSConfig() (*tls.Config, error)
	PersistentKey() any
}

// Configuration for various trust strategies. At least one field must be
// non-nil. The first non-nil field encountered (in order) will be used to
// configure the trust strategy.
type StrategyConfig struct {
	PKP      *PKPConfig
	CACerts  *CACertsConfig
	Insecure *InsecureConfig
}

type source[T any] interface {
	Items() ([]T, error)
}

type (
	PinSource     source[*pkp.PublicKeyPin]
	CACertsSource source[*x509.Certificate]
)

type sourceFunc[T any] func() ([]T, error)

func (f sourceFunc[T]) Items() ([]T, error) {
	return f()
}

func NewKeyringPinSource(kr keyring.Keyring) PinSource {
	return sourceFunc[*pkp.PublicKeyPin](func() ([]*pkp.PublicKeyPin, error) {
		var pkpKey *keyring.PKPKey
		var err error
		kr.Try(func(pk *keyring.PKPKey) {
			if pkpKey != nil {
				err = errors.New("keyring contains multiple PKP key sets")
			}
			pkpKey = pk
		})
		if err != nil {
			return nil, err
		}
		if pkpKey == nil {
			return nil, errors.New("keyring is missing PKP key")
		}
		return pkpKey.PinnedKeys, nil
	})
}

func NewPinSource(pins []*pkp.PublicKeyPin) PinSource {
	return sourceFunc[*pkp.PublicKeyPin](func() ([]*pkp.PublicKeyPin, error) {
		return pins, nil
	})
}

func NewKeyringCACertsSource(kr keyring.Keyring) CACertsSource {
	return sourceFunc[*x509.Certificate](func() ([]*x509.Certificate, error) {
		var caCertsKey *keyring.CACertsKey
		var err error
		kr.Try(func(ca *keyring.CACertsKey) {
			if caCertsKey != nil {
				err = errors.New("keyring contains multiple CA certs key sets")
			}
			caCertsKey = ca
		})
		if err != nil {
			return nil, err
		}
		if caCertsKey == nil {
			return nil, errors.New("keyring is missing CA certs key")
		}
		certs := make([]*x509.Certificate, len(caCertsKey.CACerts))
		for i, der := range caCertsKey.CACerts {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CA cert: %w", err)
			}
			certs[i] = c
		}
		return certs, nil
	})
}

func NewCACertsSource(certs []*x509.Certificate) CACertsSource {
	return sourceFunc[*x509.Certificate](func() ([]*x509.Certificate, error) {
		return certs, nil
	})
}

type (
	PKPConfig struct {
		Pins PinSource
	}
	CACertsConfig struct {
		// If nil, system certs will be used.
		CACerts CACertsSource
	}
	InsecureConfig struct {
	}
)

func (c *StrategyConfig) Build() (Strategy, error) {
	switch {
	case c.PKP != nil:
		if c.PKP.Pins == nil {
			return nil, fmt.Errorf("no pin source provided for PKP trust strategy")
		}
		pins, err := c.PKP.Pins.Items()
		if err != nil {
			return nil, err
		}
		return &pkpTrustStrategy{
			pins: pins,
		}, nil
	case c.CACerts != nil:
		var certs []*x509.Certificate
		if c.CACerts.CACerts != nil {
			var err error
			certs, err = c.CACerts.CACerts.Items()
			if err != nil {
				return nil, err
			}
		}
		return &caCertsTrustStrategy{
			caCerts: certs,
		}, nil
	case c.Insecure != nil:
		return &insecureTrustStrategy{}, nil
	default:
		return nil, fmt.Errorf("no trust strategy configured")
	}
}

type (
	pkpTrustStrategy struct {
		pins []*pkp.PublicKeyPin
	}
	caCertsTrustStrategy struct {
		caCerts []*x509.Certificate
	}
	insecureTrustStrategy struct {
	}
)

func (p *pkpTrustStrategy) PersistentKey() any {
	return keyring.NewPKPKey(p.pins)
}

func (c *caCertsTrustStrategy) PersistentKey() any {
	return keyring.NewCACertsKey(c.caCerts)
}

func (p *pkpTrustStrategy) TLSConfig() (*tls.Config, error) {
	return pkp.TLSConfig(p.pins)
}

func (c *caCertsTrustStrategy) TLSConfig() (*tls.Config, error) {
	var pool *x509.CertPool
	if len(c.caCerts) > 0 {
		pool = x509.NewCertPool()
		for _, cert := range c.caCerts {
			pool.AddCert(cert)
		}
	} else {
		var err error
		pool, err = x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
	}

	return &tls.Config{
		RootCAs: pool,
	}, nil
}

func (i *insecureTrustStrategy) TLSConfig() (*tls.Config, error) {
	return &tls.Config{
		InsecureSkipVerify: true,
	}, nil
}

func (i *insecureTrustStrategy) PersistentKey() any {
	return nil
}

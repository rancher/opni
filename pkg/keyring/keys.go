package keyring

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
)

// Key types are used indirectly via an interface, as most key values would
// benefit from extra accessor logic (e.g. copying raw byte arrays).

type SharedKeys struct {
	ClientKey ed25519.PrivateKey `json:"clientKey"`
	ServerKey ed25519.PrivateKey `json:"serverKey"`
}

type TLSKey struct {
	TLSConfig *TLSConfig `json:"tlsConfig"`
}

func NewSharedKeys(secret []byte) *SharedKeys {
	if len(secret) != 64 {
		panic("shared secret must be 64 bytes")
	}
	return &SharedKeys{
		ClientKey: ed25519.NewKeyFromSeed(secret[:32]),
		ServerKey: ed25519.NewKeyFromSeed(secret[32:]),
	}
}

func NewTLSKeys(tls *TLSConfig) *TLSKey {
	return &TLSKey{
		TLSConfig: tls,
	}
}

// TLSConfig is a json-encodable TLS config, only containing fields we need.
type TLSConfig struct {
	Certificates     []tls.Certificate   `json:"certificates,omitempty"`
	CurvePreferences []tls.CurveID       `json:"curvePreferences,omitempty"`
	RootCAs          []*x509.Certificate `json:"rootCAs,omitempty"`
	ServerName       string              `json:"serverName,omitempty"`
}

func (t *TLSConfig) ToCryptoTLSConfig() *tls.Config {
	rootCAs := x509.NewCertPool()
	for _, cert := range t.RootCAs {
		rootCAs.AddCert(cert)
	}
	return &tls.Config{
		Certificates:     t.Certificates,
		CurvePreferences: t.CurvePreferences,
		RootCAs:          rootCAs,
		ServerName:       t.ServerName,
	}
}

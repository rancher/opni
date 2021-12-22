package keyring

import (
	"crypto/ed25519"
	"crypto/tls"
)

// Key types are used indirectly via an interface, as most key values would
// benefit from extra accessor logic (e.g. copying raw byte arrays).

type ClientKeys interface {
	ClientKey() ed25519.PrivateKey
	TenantKey() ed25519.PrivateKey
}

type TLSKey interface {
	TLSConfig() *tls.Config
}

type clientKeys struct {
	Client ed25519.PrivateKey `json:"client_key"`
	Tenant ed25519.PrivateKey `json:"tenant_key"`
}

type tlsKey struct {
	Tls *tls.Config `json:"tls"`
}

func NewClientKeys(secret []byte) ClientKeys {
	if len(secret) != 64 {
		panic("shared secret must be 64 bytes")
	}
	return &clientKeys{
		Client: ed25519.NewKeyFromSeed(secret[:32]),
		Tenant: ed25519.NewKeyFromSeed(secret[32:]),
	}
}

func NewTLSKeys(tls *tls.Config) TLSKey {
	return &tlsKey{
		Tls: tls,
	}
}

func (k *clientKeys) ClientKey() ed25519.PrivateKey {
	buf := make([]byte, ed25519.PrivateKeySize)
	copy(buf, k.Client)
	return buf
}

func (k *clientKeys) TenantKey() ed25519.PrivateKey {
	buf := make([]byte, ed25519.PrivateKeySize)
	copy(buf, k.Tenant)
	return buf
}

func (kr *tlsKey) TLSConfig() *tls.Config {
	return kr.Tls
}

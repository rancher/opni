package keyring

import (
	"crypto/x509"

	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/pkp"
	"golang.org/x/exp/slices"
)

// Key types are used indirectly via an interface, as most key values would
// benefit from extra accessor logic (e.g. copying raw byte arrays).

type SharedKeys struct {
	ClientKey []byte `json:"clientKey"`
	ServerKey []byte `json:"serverKey"`
}

type PKPKey struct {
	PinnedKeys []*pkp.PublicKeyPin `json:"pinnedKeys"`
}

type CACertsKey struct {
	// DER-encoded certificates
	CACerts [][]byte `json:"caCerts"`
}

type EphemeralKey = ephemeral.Key

func NewSharedKeys(secret []byte) *SharedKeys {
	if len(secret) != 64 {
		panic("shared secret must be 64 bytes")
	}
	return &SharedKeys{
		ClientKey: secret[:32],
		ServerKey: secret[32:],
	}
}

func NewPKPKey(pinnedKeys []*pkp.PublicKeyPin) *PKPKey {
	key := &PKPKey{
		PinnedKeys: make([]*pkp.PublicKeyPin, len(pinnedKeys)),
	}
	for i, pinnedKey := range pinnedKeys {
		key.PinnedKeys[i] = pinnedKey.DeepCopy()
	}
	return key
}

func NewCACertsKey(certs []*x509.Certificate) *CACertsKey {
	key := &CACertsKey{
		CACerts: make([][]byte, len(certs)),
	}
	for i, cert := range certs {
		key.CACerts[i] = slices.Clone(cert.Raw)
	}
	return key
}

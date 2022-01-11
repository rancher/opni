package keyring

import (
	"crypto/ed25519"

	"github.com/kralicky/opni-monitoring/pkg/pkp"
)

// Key types are used indirectly via an interface, as most key values would
// benefit from extra accessor logic (e.g. copying raw byte arrays).

type SharedKeys struct {
	ClientKey ed25519.PrivateKey `json:"clientKey"`
	ServerKey ed25519.PrivateKey `json:"serverKey"`
}

type PKPKey struct {
	PinnedKeys []*pkp.PublicKeyPin `json:"pinnedKeys"`
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

func NewPKPKey(pinnedKeys []*pkp.PublicKeyPin) *PKPKey {
	key := &PKPKey{
		PinnedKeys: make([]*pkp.PublicKeyPin, len(pinnedKeys)),
	}
	for i, pinnedKey := range pinnedKeys {
		key.PinnedKeys[i] = pinnedKey.DeepCopy()
	}
	return key
}

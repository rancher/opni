package ecdh

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/curve25519"
)

type EphemeralKeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

type ClientKeyring struct {
	clientKey ed25519.PrivateKey
	tenantKey ed25519.PrivateKey
}

type PeerType int

const (
	PeerTypeClient PeerType = iota
	PeerTypeServer
)

type PeerPublicKey struct {
	PublicKey []byte
	PeerType  PeerType
}

// Creates a new x25519 keypair for use in ECDH key exchange.
func NewEphemeralKeyPair() (EphemeralKeyPair, error) {
	priv, err := io.ReadAll(io.LimitReader(rand.Reader, 32))
	if err != nil {
		return EphemeralKeyPair{}, err
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return EphemeralKeyPair{}, err
	}
	return EphemeralKeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}, nil
}

// Derives a 64-byte shared secret given one party's ephemeral keypair and
// another party's ephemeral public key obtained from ECDH.
// The secret is computed using the following KDF (similar to libsodium):
//  blake2b-512(q || client-pub || server-pub).
// where q is the 32-byte x25519 shared secret.
//
// The client and server's public keys must be ordered the same way on both
// sides, so the peer's type (client or server) must be provided along with
// the peer's public key.
func DeriveSharedSecret(ours EphemeralKeyPair, theirs PeerPublicKey) ([]byte, error) {
	q, err := curve25519.X25519(ours.PrivateKey, theirs.PublicKey)
	if err != nil {
		return nil, err
	}
	hash, _ := blake2b.New512(nil)
	hash.Write(q)
	switch theirs.PeerType {
	case PeerTypeClient:
		hash.Write(theirs.PublicKey)
		hash.Write(ours.PublicKey)
	case PeerTypeServer:
		hash.Write(ours.PublicKey)
		hash.Write(theirs.PublicKey)
	}

	return hash.Sum(nil), nil
}

func GenerateClientKeyring(shared []byte) *ClientKeyring {
	if len(shared) != 64 {
		panic("shared secret must be 64 bytes")
	}
	return &ClientKeyring{
		clientKey: ed25519.NewKeyFromSeed(shared[:32]),
		tenantKey: ed25519.NewKeyFromSeed(shared[32:]),
	}
}

func (kr *ClientKeyring) ClientKey() ed25519.PrivateKey {
	buf := make([]byte, ed25519.PrivateKeySize)
	copy(buf, kr.clientKey)
	return buf
}

func (kr *ClientKeyring) TenantKey() ed25519.PrivateKey {
	buf := make([]byte, ed25519.PrivateKeySize)
	copy(buf, kr.tenantKey)
	return buf
}

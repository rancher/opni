package ecdh

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/curve25519"
)

var (
	ErrInvalidPeerType = errors.New("invalid peer type")
)

type EphemeralKeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
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
func NewEphemeralKeyPair() EphemeralKeyPair {
	priv, err := io.ReadAll(io.LimitReader(rand.Reader, 32))
	if err != nil {
		panic(err)
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		panic(err)
	}
	return EphemeralKeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}
}

// Derives a 64-byte shared secret given one party's ephemeral keypair and
// another party's ephemeral public key obtained from ECDH.
//
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
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidPeerType, theirs.PeerType)
	}

	return hash.Sum(nil), nil
}

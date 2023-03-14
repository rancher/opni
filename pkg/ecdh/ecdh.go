package ecdh

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

var (
	ErrInvalidPeerType = errors.New("invalid peer type")
)

type EphemeralKeyPair struct {
	PrivateKey *ecdh.PrivateKey
	PublicKey  *ecdh.PublicKey
}

type PeerType int

const (
	PeerTypeClient PeerType = iota
	PeerTypeServer
)

type PeerPublicKey struct {
	PublicKey *ecdh.PublicKey
	PeerType  PeerType
}

// Creates a new x25519 keypair for use in ECDH key exchange.
func NewEphemeralKeyPair() EphemeralKeyPair {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	pub := priv.PublicKey()
	return EphemeralKeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
	}
}

// Derives a 64-byte shared secret given one party's ephemeral keypair and
// another party's ephemeral public key obtained from ECDH.
//
// The secret is computed using the following KDF (similar to libsodium):
//
//	blake2b-512(q || client-pub || server-pub).
//
// where q is the 32-byte x25519 shared secret.
//
// The client and server's public keys must be ordered the same way on both
// sides, so the peer's type (client or server) must be provided along with
// the peer's public key.
func DeriveSharedSecret(ours EphemeralKeyPair, theirs PeerPublicKey) ([]byte, error) {
	q, err := ours.PrivateKey.ECDH(theirs.PublicKey)
	if err != nil {
		return nil, err
	}
	hash, _ := blake2b.New512(nil)
	hash.Write(q)
	switch theirs.PeerType {
	case PeerTypeClient:
		hash.Write(theirs.PublicKey.Bytes())
		hash.Write(ours.PublicKey.Bytes())
	case PeerTypeServer:
		hash.Write(ours.PublicKey.Bytes())
		hash.Write(theirs.PublicKey.Bytes())
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidPeerType, theirs.PeerType)
	}

	return hash.Sum(nil), nil
}

type clientGetter interface {
	GetClientPubKey() []byte
}

type serverGetter interface {
	GetServerPubKey() []byte
}

func ClientPubKey[T clientGetter](t T) (PeerPublicKey, error) {
	pubKey, err := ecdh.X25519().NewPublicKey(t.GetClientPubKey())
	if err != nil {
		return PeerPublicKey{}, err
	}
	return PeerPublicKey{
		PublicKey: pubKey,
		PeerType:  PeerTypeClient,
	}, nil
}

func ServerPubKey[T serverGetter](t T) (PeerPublicKey, error) {
	pubKey, err := ecdh.X25519().NewPublicKey(t.GetServerPubKey())
	if err != nil {
		return PeerPublicKey{}, err
	}
	return PeerPublicKey{
		PublicKey: pubKey,
		PeerType:  PeerTypeServer,
	}, nil
}

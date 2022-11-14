package b2mac

import (
	"crypto/ed25519"
	"crypto/subtle"
	"errors"

	"golang.org/x/crypto/blake2b"

	"github.com/google/uuid"
)

// Computes a blake2b-512 MAC for the given tenant ID and message payload
// using the provided private key and random nonce.
// This function will only return an error if there is a problem with the
// private key.
func New512(id []byte, nonce uuid.UUID, payload []byte, key ed25519.PrivateKey) ([]byte, error) {
	mac, err := blake2b.New512(key)
	if err != nil {
		return nil, err
	}
	mac.Write(id)
	mac.Write(nonce[:])
	mac.Write(payload)
	return mac.Sum(nil), nil
}

func Verify(mac []byte, id []byte, nonce uuid.UUID, payload []byte, key ed25519.PrivateKey) error {
	m, err := blake2b.New512(key)
	if err != nil {
		return err
	}
	m.Write(id)
	m.Write(nonce[:])
	m.Write(payload)
	if subtle.ConstantTimeCompare(m.Sum(nil), mac) == 1 {
		return nil
	}
	return errors.New("verification failed")
}

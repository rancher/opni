package crypto

import (
	"errors"

	"golang.org/x/crypto/sha3"
)

type PRFXOFHasher interface {
	Hash(data []byte, outputLen int) ([]byte, error)
}

type cshakeHasher struct {
	domain []byte
	key    []byte
}

func NewCShakeHasher(secretKey []byte, domain string) PRFXOFHasher {
	return &cshakeHasher{
		domain: []byte(domain),
		key:    secretKey,
	}
}

func (c *cshakeHasher) Hash(data []byte, outputLen int) ([]byte, error) {
	if outputLen < 32 {
		return nil, errors.New("invalid output length, must be at least 32 bytes")
	}
	d := sha3.NewCShake256(nil, c.domain)

	d.Write(c.key)
	d.Write(data)

	h := make([]byte, outputLen)
	d.Read(h)
	return h, nil
}

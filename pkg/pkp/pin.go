package pkp

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

var (
	ErrMissingAlgorithm     = errors.New("missing algorithm")
	ErrUnsupportedAlgorithm = errors.New("unsupported algorithm")
	ErrMalformedPin         = errors.New("malformed pin")
)

type Alg string

const (
	AlgSHA256 Alg = "sha256"
	AlgB2B256 Alg = "b2b256"
)

type PublicKeyPin struct {
	Algorithm   Alg    `json:"alg"`
	Fingerprint []byte `json:"fingerprint"`
}

func (p *PublicKeyPin) DeepCopy() *PublicKeyPin {
	return &PublicKeyPin{
		Algorithm:   p.Algorithm,
		Fingerprint: append([]byte{}, p.Fingerprint...),
	}
}

func (p *PublicKeyPin) Validate() error {
	switch p.Algorithm {
	case AlgSHA256, AlgB2B256:
	default:
		return ErrUnsupportedAlgorithm
	}
	if len(p.Fingerprint) != 32 {
		return ErrMalformedPin
	}
	return nil
}

func (p *PublicKeyPin) Encode() string {
	return fmt.Sprintf("%s:%s", p.Algorithm, base64.RawURLEncoding.EncodeToString(p.Fingerprint))
}

func (p *PublicKeyPin) Equal(other *PublicKeyPin) bool {
	return p.Algorithm == other.Algorithm &&
		subtle.ConstantTimeCompare(p.Fingerprint, other.Fingerprint) == 1
}

func NewBlake2b256(cert *x509.Certificate) *PublicKeyPin {
	d := blake2b.Sum256(cert.RawSubjectPublicKeyInfo)
	return &PublicKeyPin{
		Algorithm:   AlgB2B256,
		Fingerprint: d[:],
	}
}

func NewSha256(cert *x509.Certificate) *PublicKeyPin {
	d := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return &PublicKeyPin{
		Algorithm:   AlgSHA256,
		Fingerprint: d[:],
	}
}

func New(cert *x509.Certificate, alg Alg) (*PublicKeyPin, error) {
	switch alg {
	case AlgSHA256:
		return NewSha256(cert), nil
	case AlgB2B256:
		return NewBlake2b256(cert), nil
	default:
		return nil, ErrUnsupportedAlgorithm
	}
}

func DecodePin(pin string) (*PublicKeyPin, error) {
	parts := strings.Split(pin, ":")
	if len(parts) == 1 {
		return nil, ErrMissingAlgorithm
	} else if len(parts) > 2 {
		return nil, ErrMalformedPin
	}

	fp, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMalformedPin, err)
	}
	switch Alg(parts[0]) {
	case AlgSHA256:
		return &PublicKeyPin{
			Algorithm:   AlgSHA256,
			Fingerprint: fp,
		}, nil
	case AlgB2B256:
		return &PublicKeyPin{
			Algorithm:   AlgB2B256,
			Fingerprint: fp,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, parts[0])
	}
}

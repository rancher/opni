// Package session implements session attributes for stream connections.
package session

import (
	"context"
	"crypto/subtle"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

type (
	sessionAttributesKeyType string
)

const (
	AttributeMetadataKey = "x-session-attribute"
	AttributeLabelKey    = "opni.io/session-attribute"

	KeyLength = 32
)

const (
	AttributesKey sessionAttributesKeyType = "session_attributes"
)

func StreamAuthorizedAttributes(ctx context.Context) []Attribute {
	attrs, ok := ctx.Value(AttributesKey).([]Attribute)
	if !ok {
		return nil
	}
	return attrs
}

type Attribute interface {
	// Name returns the attribute name. This should be a unique identifier.
	Name() string

	// Given a challenge string, Solve will compute a MAC using an
	// implementation-specific secret key and return the result.
	Solve(id string, challenge []byte) []byte

	// Verify will check if a mac is valid for a given id and challenge.
	Verify(id string, challenge []byte, response []byte) bool
}

type attribute struct {
	name string

	key [KeyLength]byte
}

var _ Attribute = (*attribute)(nil)

// Name implements Attribute
func (a *attribute) Name() string {
	return a.name
}

// Solve implements Attribute
func (a *attribute) Solve(id string, challenge []byte) []byte {
	mac, _ := blake2b.New512(a.key[:])
	mac.Write([]byte(a.name))
	mac.Write([]byte(id))
	mac.Write(challenge[:])
	return mac.Sum(nil)
}

// Verify implements Attribute
func (a *attribute) Verify(id string, challenge []byte, response []byte) bool {
	expectedMac, _ := blake2b.New512(a.key[:])
	expectedMac.Write([]byte(a.name))
	expectedMac.Write([]byte(id))
	expectedMac.Write(challenge[:])
	expected := expectedMac.Sum(nil)
	return subtle.ConstantTimeCompare(expected, response) == 1
}

func NewAttribute(name string, key []byte) (Attribute, error) {
	if len(key) != KeyLength {
		return nil, fmt.Errorf("invalid key length: %d", len(key))
	}
	return &attribute{
		name: name,
		key:  [KeyLength]byte(key),
	}, nil
}

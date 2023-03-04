// Package session implements session attributes for stream connections.
package session

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc/metadata"
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

func (k sessionAttributesKeyType) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if v := ctx.Value(k); v != nil {
		var names []string
		switch v := v.(type) {
		case []Attribute:
			names = attributeNames(v)
		case []SecretAttribute:
			names = attributeNames(v)
		default:
			return nil, fmt.Errorf("invalid session attribute type %T", v)
		}
		return map[string]string{
			string(k): strings.Join(names, " "),
		}, nil
	}
	return nil, nil
}

func (k sessionAttributesKeyType) RequireTransportSecurity() bool {
	return false
}

func (k sessionAttributesKeyType) FromIncomingCredentials(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	if values := md.Get(string(k)); len(values) == 1 {
		names := strings.Split(values[0], " ")
		attrs := make([]Attribute, len(names))
		for i, name := range names {
			attrs[i] = &attribute{name: name}
		}
		ctx = context.WithValue(ctx, k, attrs)
	}
	return ctx
}

func StreamAuthorizedAttributes(ctx context.Context) []Attribute {
	attrs := ctx.Value(AttributesKey)
	if attrs == nil {
		return nil
	}
	switch attrs := attrs.(type) {
	case []Attribute:
		return attrs
	case []SecretAttribute:
		return toAttributeSlice(attrs)
	default:
		return nil
	}
}

type Attribute interface {
	// Name returns the attribute name. This should be a unique identifier.
	Name() string
}

type SecretAttribute interface {
	Attribute

	// Given a challenge string, Solve will compute a MAC using an
	// implementation-specific secret key and return the result.
	Solve(id string, challenge []byte) []byte

	// Verify will check if a mac is valid for a given id and challenge.
	Verify(id string, challenge []byte, response []byte) bool
}

func toAttributeSlice[T Attribute](attrs []T) []Attribute {
	result := make([]Attribute, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}
	return result
}

func attributeNames[T Attribute](attrs []T) []string {
	names := make([]string, len(attrs))
	for i, attr := range attrs {
		names[i] = attr.Name()
	}
	return names
}

type attribute struct {
	name string
}

type secretAttribute struct {
	attribute

	key [KeyLength]byte
}

var _ Attribute = (*attribute)(nil)
var _ SecretAttribute = (*secretAttribute)(nil)

// Name implements Attribute
func (a *attribute) Name() string {
	return a.name
}

// Solve implements Attribute
func (a *secretAttribute) Solve(id string, challenge []byte) []byte {
	mac, _ := blake2b.New512(a.key[:])
	mac.Write([]byte(a.name))
	mac.Write([]byte(id))
	mac.Write(challenge[:])
	return mac.Sum(nil)
}

// Verify implements Attribute
func (a *secretAttribute) Verify(id string, challenge []byte, response []byte) bool {
	expectedMac, _ := blake2b.New512(a.key[:])
	expectedMac.Write([]byte(a.name))
	expectedMac.Write([]byte(id))
	expectedMac.Write(challenge[:])
	expected := expectedMac.Sum(nil)
	return subtle.ConstantTimeCompare(expected, response) == 1
}

func NewSecretAttribute(name string, key []byte) (SecretAttribute, error) {
	if len(key) != KeyLength {
		return nil, fmt.Errorf("invalid key length: %d", len(key))
	}
	return &secretAttribute{
		attribute: attribute{
			name: name,
		},
		key: [KeyLength]byte(key),
	}, nil
}

func NewAttribute(name string) Attribute {
	return &attribute{name: name}
}

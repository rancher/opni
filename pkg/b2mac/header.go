package b2mac

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Generates the value of the "Authorization" header for a MAC with the
// given id, nonce, and mac.
// The id is an arbitrary string and should correspond to the tenant ID.
// The nonce should be a v4 UUID.
// The mac should not have any encoding.
func EncodeAuthHeader(id []byte, nonce uuid.UUID, mac []byte) (string, error) {
	if nonce.Version() != 4 || nonce.Variant() != uuid.RFC4122 {
		return "", errors.New("nonce is not a v4 UUID")
	}
	macEncoded := base64.RawURLEncoding.EncodeToString(mac)
	idEncoded := base64.RawURLEncoding.EncodeToString(id)
	return fmt.Sprintf(`MAC id="%s",nonce="%s",mac="%s"`, idEncoded, nonce.String(), macEncoded), nil
}

// Decodes the value of the "Authorization" header into its constituent parts.
// It returns an ID (the tenant ID), a nonce (a v4 UUID), and an unencoded MAC.
func DecodeAuthHeader(header string) (id []byte, nonce uuid.UUID, mac []byte, err error) {
	if !strings.HasPrefix(header, "MAC ") {
		return nil, uuid.Nil, nil, errors.New("incorrect authorization type")
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(header, "MAC"))
	kvPairs := strings.Split(trimmed, ",")
	foundKeys := map[string]struct{}{}
	for _, pair := range kvPairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return nil, uuid.Nil, nil, errors.New("malformed key-value pair")
		}
		if len(kv[1]) < 2 || kv[1][0] != '"' || kv[1][len(kv[1])-1] != '"' {
			return nil, uuid.Nil, nil, errors.New("expected quoted string")
		} else {
			kv[1] = kv[1][1 : len(kv[1])-1]
		}
		key := strings.TrimSpace(kv[0])
		if _, ok := foundKeys[key]; ok {
			return nil, uuid.Nil, nil, errors.New("duplicate key: " + key)
		}
		foundKeys[key] = struct{}{}
		switch key {
		case "id":
			id, err = base64.RawURLEncoding.DecodeString(kv[1])
			if err != nil {
				return nil, uuid.Nil, nil, errors.New("malformed id")
			}
		case "nonce":
			nonce, err = uuid.Parse(kv[1])
			if err != nil {
				return nil, uuid.Nil, nil, err
			}
			if nonce.Version() != 4 || nonce.Variant() != uuid.RFC4122 {
				return nil, uuid.Nil, nil, errors.New("nonce is not a v4 UUID")
			}
		case "mac":
			mac, err = base64.RawURLEncoding.DecodeString(kv[1])
			if err != nil {
				return nil, uuid.Nil, nil, err
			}
		default:
			return nil, uuid.Nil, nil, errors.New("unknown key")
		}
	}
	if len(id) == 0 {
		err = fmt.Errorf("Header is missing id")
	}
	if nonce == uuid.Nil {
		err = fmt.Errorf("Header is missing nonce")
	}
	if len(mac) == 0 {
		err = fmt.Errorf("Header is missing signature")
	}
	return
}

func NewEncodedHeader(id []byte, nonce uuid.UUID, payload []byte, key ed25519.PrivateKey) (string, error) {
	mac, err := New512(id, nonce, payload, key)
	if err != nil {
		return "", err
	}
	return EncodeAuthHeader(id, nonce, mac)
}

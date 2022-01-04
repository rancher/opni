package b2bmac

import (
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
func EncodeAuthHeader(id string, nonce uuid.UUID, mac []byte) (string, error) {
	if nonce.Variant() != uuid.RFC4122 {
		return "", errors.New("nonce is not a v4 UUID")
	}
	encoded := base64.RawURLEncoding.EncodeToString(mac)
	return fmt.Sprintf(`MAC id="%s",nonce="%s",mac="%s"`, id, nonce.String(), encoded), nil
}

// Decodes the value of the "Authorization" header into its constituent parts.
// It returns an ID (the tenant ID), a nonce (a v4 UUID), and an unencoded MAC.
func DecodeAuthHeader(header string) (id string, nonce uuid.UUID, mac []byte, err error) {
	if !strings.HasPrefix(header, "MAC ") {
		return "", uuid.Nil, nil, errors.New("incorrect authorization type")
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(header, "MAC"))
	kvPairs := strings.Split(trimmed, ",")
	for _, pair := range kvPairs {
		kv := strings.Split(pair, "=")
		switch strings.TrimSpace(kv[0]) {
		case "id":
			id = strings.Trim(kv[1], `"`)
		case "nonce":
			nonce, err = uuid.Parse(strings.Trim(kv[1], `"`))
			if err != nil {
				return "", uuid.Nil, nil, err
			}
			if nonce.Variant() != uuid.RFC4122 {
				return "", uuid.Nil, nil, errors.New("nonce is not a v4 UUID")
			}
		case "mac":
			mac, err = base64.RawURLEncoding.DecodeString(strings.Trim(kv[1], `"`))
			if err != nil {
				return "", uuid.Nil, nil, err
			}
		}
	}
	if id == "" {
		err = fmt.Errorf("Header is missing id")
	}
	if nonce == uuid.Nil {
		err = fmt.Errorf("Header is missing nonce")
	}
	if mac == nil {
		err = fmt.Errorf("Header is missing signature")
	}
	return
}

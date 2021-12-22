package keyring

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"reflect"
)

var (
	ErrInvalidKeyType            = errors.New("invalid key type")
	ErrSignatureValidationFailed = errors.New("signature validation failed")
)

var allowedKeyTypes = map[reflect.Type]struct{}{
	reflect.TypeOf(&clientKeys{}): {},
	reflect.TypeOf(&tlsKey{}):     {},
}

type UseKeyFn func(key interface{})

type Keyring interface {
	Try(...UseKeyFn)
	ForEach(func(key interface{}))
	Marshal(signer ed25519.PrivateKey) ([]byte, error)
}

type keyring struct {
	Keys map[reflect.Type]interface{}
}

func New(keys ...interface{}) Keyring {
	kr := &keyring{
		Keys: make(map[reflect.Type]interface{}),
	}
	m := map[reflect.Type]interface{}{}
	for _, key := range keys {
		t := reflect.TypeOf(key)
		if _, ok := allowedKeyTypes[t]; !ok {
			panic(ErrInvalidKeyType)
		}
		m[t] = key
	}
	return kr
}

func (kr *keyring) Try(fns ...UseKeyFn) {
	for _, fn := range fns {
		fnType := reflect.TypeOf(fn)
		argType := fnType.In(0)
		if k, ok := kr.Keys[argType]; ok {
			fn(k)
		}
	}
}

func (kr *keyring) ForEach(fn func(key interface{})) {
	for _, k := range kr.Keys {
		fn(k)
	}
}

type signedKeyring struct {
	Keyring   []byte `json:"keyring"`
	Signature []byte `json:"signature,omitempty"`
}

// Marshal the keyring to JSON. If signer is not nil, the keyring will be
// signed with the private key.
func (kr *keyring) Marshal(signer ed25519.PrivateKey) ([]byte, error) {
	keys := map[string][]byte{}
	for t, k := range kr.Keys {
		name := t.PkgPath() + "." + t.Name()
		data, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		keys[name] = data
	}
	encodedKeys, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	signedKr := signedKeyring{
		Keyring:   encodedKeys,
		Signature: nil,
	}
	if signer != nil {
		sig, err := signer.Sign(rand.Reader, encodedKeys, nil)
		if err != nil {
			return nil, err
		}
		signedKr.Signature = sig
	}
	return json.Marshal(signedKr)
}

// Unmarshal a keyring which was previously marshaled using keyring.Marshal.
// If pub is not nil, the keyring's signature will be validated against the
// public key. If pub is nil and the keyring has a signature, or if pub is
// not nil and the keyring does not have a signature, this will be treated
// as a validation failure.
func Unmarshal(data []byte, kr Keyring, pub ed25519.PublicKey) (Keyring, error) {
	signed := &signedKeyring{}
	err := json.Unmarshal(data, signed)
	if err != nil {
		return nil, err
	}
	hasSignature := (signed.Signature != nil)
	hasPubKey := (pub != nil)
	if hasSignature != hasPubKey {
		// if signature == nil xor pub == nil, fail validation
		return nil, ErrSignatureValidationFailed
	}
	if hasSignature && hasPubKey {
		if !ed25519.Verify(pub, signed.Keyring, signed.Signature) {
			return nil, ErrSignatureValidationFailed
		}
	}
	encodedKeys := map[string][]byte{}
	err = json.Unmarshal(signed.Keyring, &encodedKeys)
	if err != nil {
		return nil, err
	}
	keys := []interface{}{}
	for name, data := range encodedKeys {
		for allowedType := range allowedKeyTypes {
			if name == allowedType.PkgPath()+"."+allowedType.Name() {
				k := reflect.New(allowedType).Interface()
				err := json.Unmarshal(data, k)
				if err != nil {
					return nil, err
				}
				keys = append(keys, k)
			}
		}
	}
	return New(keys...), nil
}

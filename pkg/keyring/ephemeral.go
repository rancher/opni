package keyring

import (
	"encoding/json"
	"io"

	"github.com/go-playground/validator/v10"
)

type UsageType string

const (
	Encryption UsageType = "encryption"
	Signing    UsageType = "signing"
)

var validate = validator.New()

type EphemeralKey struct {
	Usage  UsageType         `json:"usage" validate:"oneof=encryption signing"`
	Secret []byte            `json:"secret" validate:"len=32"`
	Labels map[string]string `json:"labels" validate:"dive,keys,printascii,endkeys,printascii"`
}

func (e *EphemeralKey) Validate() error {
	if err := validate.Struct(e); err != nil {
		return err
	}
	return nil
}

func LoadEphemeralKey(r io.Reader) (*EphemeralKey, error) {
	var key EphemeralKey
	if err := json.NewDecoder(r).Decode(&key); err != nil {
		return nil, err
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	return &key, nil
}

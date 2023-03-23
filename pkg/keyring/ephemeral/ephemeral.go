package ephemeral

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"github.com/rancher/opni/pkg/validation"
)

type (
	KeyUsageType string
)

const (
	Authentication KeyUsageType = "auth"
)

type Key struct {
	Labels map[string]string `json:"labels"`
	Usage  KeyUsageType      `json:"usage"`
	Secret []byte            `json:"sec,omitempty"`
}

func (e *Key) Validate() error {
	if err := validation.ValidateLabels(e.Labels); err != nil {
		return err
	}
	switch e.Usage {
	case Authentication:
	default:
		return validation.Errorf("invalid usage type: %s", e.Usage)
	}
	if len(e.Secret) < 16 {
		return validation.Errorf("secret key must be at least 128 bits")
	}
	return nil
}

func LoadKey(r io.Reader) (*Key, error) {
	var key Key
	if err := json.NewDecoder(r).Decode(&key); err != nil {
		return nil, err
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	return &key, nil
}

func NewKey(usage KeyUsageType, labels map[string]string) *Key {
	ek := &Key{
		Usage:  usage,
		Labels: labels,
	}
	switch usage {
	case Authentication:
		var key [32]byte
		rand.Read(key[:])
		ek.Secret = key[:]
	default:
		panic(fmt.Errorf("unknown usage type: %s", usage))
	}
	return ek
}

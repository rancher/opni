package bootstrap

import (
	"context"

	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/validation"
)

type Bootstrapper interface {
	Bootstrap(context.Context, ident.Provider) (keyring.Keyring, error)
	Finalize(context.Context) error
}

type BootstrapJoinResponse struct {
	Signatures map[string][]byte `json:"signatures"`
}

type BootstrapAuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientPubKey []byte `json:"client_pub_key"`
	Capability   string `json:"capability"`
}

type BootstrapAuthResponse struct {
	ServerPubKey []byte `json:"server_pub_key"`
}

func (h BootstrapAuthRequest) Validate() error {
	if h.ClientID == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "client_id")
	}
	if err := validation.ValidateID(h.ClientID); err != nil {
		return validation.ErrInvalidID
	}
	if len(h.ClientPubKey) == 0 {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "client_pub_key")
	}
	if h.Capability == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "capability")
	}
	return nil
}

package bootstrap

import (
	"context"

	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
)

type Bootstrapper interface {
	Bootstrap(context.Context, ident.Provider) (keyring.Keyring, error)
}

type BootstrapJoinResponse struct {
	Signatures map[string][]byte `json:"signatures"`
}

type BootstrapAuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientPubKey []byte `json:"client_pub_key"`
}

type BootstrapAuthResponse struct {
	ServerPubKey []byte `json:"server_pub_key"`
}

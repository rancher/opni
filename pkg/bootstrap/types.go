package bootstrap

import (
	"context"

	"github.com/kralicky/opni-gateway/pkg/keyring"
)

type Bootstrapper interface {
	Bootstrap(ctx context.Context) (keyring.Keyring, error)
}

type BootstrapResponse struct {
	CACert     []byte            `json:"ca_cert"`
	Signatures map[string][]byte `json:"signatures"`
}
type SecureBootstrapRequest struct {
	ClientID     string `json:"client_id"`
	ClientPubKey []byte `json:"client_pub_key"`
}

type SecureBootstrapResponse struct {
	ServerPubKey []byte `json:"server_pub_key"`
}

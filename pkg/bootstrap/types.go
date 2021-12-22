package bootstrap

import "crypto"

type BootstrapResponse struct {
	CACert       string            `json:"ca_cert"`
	Signatures   map[string]string `json:"signatures"`
	EphemeralKey crypto.PublicKey  `json:"ephemeral_key"`
}
type SecureBootstrapRequest struct {
	ClientID     string `json:"client_id"`
	ClientPubKey []byte `json:"ephemeral_key"`
}

type SecureBootstrapResponse struct {
	ServerPubKey []byte `json:"jwt"`
}

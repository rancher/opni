package v1beta1

import "github.com/kralicky/opni-gateway/pkg/config/meta"

type AuthProvider struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`

	Spec AuthProviderSpec `json:"spec,omitempty"`
}

type AuthProviderType string

const (
	AuthProviderOIDC AuthProviderType = "oidc"
)

type AuthProviderSpec struct {
	Type    AuthProviderType  `json:"type,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

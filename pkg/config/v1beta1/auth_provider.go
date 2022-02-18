package v1beta1

import "github.com/rancher/opni-monitoring/pkg/config/meta"

type AuthProvider struct {
	meta.TypeMeta   `json:",inline"`
	meta.ObjectMeta `json:"metadata,omitempty"`

	Spec AuthProviderSpec `json:"spec,omitempty"`
}

type AuthProviderType string

const (
	AuthProviderOpenID AuthProviderType = "openid"
	AuthProviderNoAuth AuthProviderType = "noauth"
)

type AuthProviderSpec struct {
	Type    AuthProviderType       `json:"type,omitempty"`
	Options map[string]interface{} `json:"options,omitempty"`
}

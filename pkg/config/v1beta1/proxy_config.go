package v1beta1

import (
	"github.com/kralicky/opni-gateway/pkg/config/meta"
)

type ProxyConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec ProxyConfigSpec `json:"spec,omitempty"`
}

type ProxyConfigSpec struct {
	ListenAddress    string               `json:"listenAddress,omitempty"`
	GatewayAddress   string               `json:"gatewayAddress,omitempty"`
	IdentityProvider IdentityProviderSpec `json:"identityProvider,omitempty"`
	Storage          StorageSpec          `json:"storage,omitempty"`
	Bootstrap        BootstrapSpec        `json:"bootstrap,omitempty"`
}

type IdentityProviderType string

const (
	IdentityProviderKubernetes IdentityProviderType = "kubernetes"
	IdentityProviderHostPath   IdentityProviderType = "hostPath"
)

type IdentityProviderSpec struct {
	Type    IdentityProviderType `json:"type,omitempty"`
	Options map[string]string    `json:"options,omitempty"`
}

type BootstrapSpec struct {
	Token      string `json:"token,omitempty"`
	CACertHash string `json:"caCertHash,omitempty"`
}

func (s *ProxyConfigSpec) SetDefaults() {
	if s == nil {
		return
	}
	if s.IdentityProvider.Type == "" {
		s.IdentityProvider.Type = IdentityProviderKubernetes
	}
	if s.ListenAddress == "" {
		s.ListenAddress = ":8080"
	}
}

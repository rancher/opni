package v1beta1

import (
	"github.com/kralicky/opni-monitoring/pkg/config/meta"
)

type AgentConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec AgentConfigSpec `json:"spec,omitempty"`
}

type AgentConfigSpec struct {
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

func (s *AgentConfigSpec) SetDefaults() {
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

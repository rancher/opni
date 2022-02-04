package v1beta1

import (
	"github.com/rancher/opni-monitoring/pkg/config/meta"
)

type AgentConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec AgentConfigSpec `json:"spec,omitempty"`
}

type AgentConfigSpec struct {
	ListenAddress    string         `json:"listenAddress,omitempty"`
	GatewayAddress   string         `json:"gatewayAddress,omitempty"`
	IdentityProvider string         `json:"identityProvider,omitempty"`
	Storage          StorageSpec    `json:"storage,omitempty"`
	Bootstrap        *BootstrapSpec `json:"bootstrap,omitempty"`
}

type BootstrapSpec struct {
	Token string   `json:"token,omitempty"`
	Pins  []string `json:"pins,omitempty"`
}

func (s *AgentConfigSpec) SetDefaults() {
	if s == nil {
		return
	}
	if s.IdentityProvider == "" {
		s.IdentityProvider = "kubernetes"
	}
	if s.ListenAddress == "" {
		s.ListenAddress = ":8080"
	}
}

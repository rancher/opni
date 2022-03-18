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
	Rules            *RulesSpec     `json:"rules,omitempty"`
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

type RulesSpec struct {
	Discovery DiscoverySpec `json:"discovery,omitempty"`
}

type DiscoverySpec struct {
	PrometheusRules *PrometheusRulesSpec `json:"prometheusRules,omitempty"`
	// Search interval. Defaults to "15m"
	Interval string `json:"interval,omitempty"`
}

type PrometheusRulesSpec struct {
	// Namespaces to search for rules in. If empty, will search all accessible
	// namespaces.
	SearchNamespaces []string `json:"searchNamespaces,omitempty"`
	// Kubeconfig to use for rule discovery. If nil, will use the in-cluster
	// kubeconfig.
	Kubeconfig *string `json:"kubeconfig,omitempty"`
}

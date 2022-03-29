package v1beta1

import (
	"github.com/rancher/opni-monitoring/pkg/config/meta"
)

type AgentConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec AgentConfigSpec `json:"spec,omitempty"`
}

type AgentConfigSpec struct {
	// The address which the agent will listen on for incoming connections.
	// This should be in the format "host:port" or ":port", and must not
	// include a scheme.
	ListenAddress string `json:"listenAddress,omitempty"`
	// The address of the gateway's public HTTP API. This should be of the format
	// "https://host:port". The scheme must be "https".
	GatewayAddress string `json:"gatewayAddress,omitempty"`
	// The name of the identity provider to use. Defaults to "kubernetes".
	IdentityProvider string `json:"identityProvider,omitempty"`
	// Configuration for agent keyring storage.
	Storage   StorageSpec    `json:"storage,omitempty"`
	Bootstrap *BootstrapSpec `json:"bootstrap,omitempty"`
	Rules     *RulesSpec     `json:"rules,omitempty"`
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

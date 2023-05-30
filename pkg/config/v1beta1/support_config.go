package v1beta1

import "github.com/rancher/opni/pkg/config/meta"

var SupportAgentConfigTypeMeta = meta.TypeMeta{
	APIVersion: APIVersion,
	Kind:       "SupportAgentConfig",
}

type SupportAgentConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec SupportAgentConfigSpec `json:"spec,omitempty"`
}

type SupportAgentConfigSpec struct {
	UserID         string       `json:"userID,omitempty"`
	GatewayAddress string       `json:"gatewayAddress,omitempty"`
	AuthData       AuthDataSpec `json:"auth,omitempty"`
}

type AuthDataSpec struct {
	Pins          []string          `json:"pins,omitempty"`
	CAPath        string            `json:"caFilePath,omitempty"`
	Token         string            `json:"token,omitempty"`
	TrustStrategy TrustStrategyKind `json:"trustStrategy,omitempty"`
}

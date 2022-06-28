package v1beta1

import (
	"github.com/rancher/opni/pkg/config/meta"
)

type RealtimeServerConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec RealtimeServerSpec `json:"spec,omitempty"`
}

type RealtimeServerSpec struct {
	Metrics          MetricsSpec          `json:"metrics,omitempty"`
	ManagementClient ManagementClientSpec `json:"managementClient,omitempty"`
}

type ManagementClientSpec struct {
	Address string `json:"address,omitempty"`
}

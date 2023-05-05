package noop

import (
	"context"

	"github.com/rancher/opni/pkg/agentmanifest"
	"github.com/rancher/opni/pkg/config/v1beta1"
)

type noopAgentManifestDriver struct{}

func NewNoopAgentManifestDriver() (agentmanifest.ResolveAgentManfestDriver, error) {
	return &noopAgentManifestDriver{}, nil
}

func (d *noopAgentManifestDriver) GetPath(_ context.Context, _ string) string {
	return "example.io/opni"
}

func (d *noopAgentManifestDriver) GetDigest(_ context.Context, _ string) string {
	return "thisisadigest"
}

func (d *noopAgentManifestDriver) GetScheme() string {
	return "oci"
}

func init() {
	agentmanifest.RegisterAgentManifestBuilder(v1beta1.AgentManifestResolverTypeNoop,
		func(args ...any) (agentmanifest.ResolveAgentManfestDriver, error) {
			return NewNoopAgentManifestDriver()
		},
	)
}

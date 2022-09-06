package agent

import (
	"context"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/meta"
)

type Plugin struct {
	capabilityv1.UnsafeNodeServer
	ctx context.Context
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx: ctx,
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p))
	return scheme
}

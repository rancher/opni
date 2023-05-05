package noop

import (
	"context"

	"github.com/rancher/opni/pkg/agent/upgrader"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type noopAgentUpgrader struct{}

func NewNoopAgentUpgrader() *noopAgentUpgrader {
	return &noopAgentUpgrader{}
}

func (n *noopAgentUpgrader) SyncAgent(_ context.Context, _ []*controlv1.UpdateManifestEntry) error {
	return nil
}

func (n *noopAgentUpgrader) DoUpgrade(_ context.Context) error {
	return nil
}

func init() {
	upgrader.RegisterUpgraderBuilder("noop",
		func(...any) (upgrader.AgentUpgrader, error) {
			return NewNoopAgentUpgrader(), nil
		},
	)
}

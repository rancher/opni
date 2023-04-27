package noop

import (
	"context"

	"github.com/rancher/opni/pkg/agent/upgrader"
)

type noopAgentUpgrader struct{}

func NewNoopAgentUpgrader() *noopAgentUpgrader {
	return &noopAgentUpgrader{}
}

func (n *noopAgentUpgrader) UpgradeRequired(_ context.Context) (bool, error) {
	return false, nil
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

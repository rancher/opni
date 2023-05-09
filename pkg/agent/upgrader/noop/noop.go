package noop

import (
	"context"

	"github.com/rancher/opni/pkg/agent/upgrader"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type NoopAgentUpgrader struct {
	manifest *controlv1.UpdateManifestEntry
}

func NewNoopAgentUpgrader() *NoopAgentUpgrader {
	return &NoopAgentUpgrader{}
}

func (n *NoopAgentUpgrader) SyncAgent(_ context.Context, entries []*controlv1.UpdateManifestEntry) error {
	n.manifest = entries[0]
	return nil
}

func (n *NoopAgentUpgrader) GetManifest() *controlv1.UpdateManifestEntry {
	return n.manifest
}

func init() {
	upgrader.RegisterUpgraderBuilder("noop",
		func(...any) (upgrader.AgentUpgrader, error) {
			return NewNoopAgentUpgrader(), nil
		},
	)
}

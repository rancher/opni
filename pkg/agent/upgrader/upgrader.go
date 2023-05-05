package upgrader

import (
	"context"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type AgentUpgrader interface {
	SyncAgent(ctx context.Context, entries []*controlv1.UpdateManifestEntry) error
}

var (
	upgraderBuilderCache = map[string]func(...any) (AgentUpgrader, error){}
)

func RegisterUpgraderBuilder[T ~string](name T, builder func(...any) (AgentUpgrader, error)) {
	upgraderBuilderCache[string(name)] = builder
}

func GetUpgraderBuilder[T ~string](name T) func(...any) (AgentUpgrader, error) {
	return upgraderBuilderCache[string(name)]
}

package upgrader

import (
	"context"

	"google.golang.org/grpc"
)

type AgentUpgrader interface {
	UpgradeRequired(context.Context) (bool, error)
	DoUpgrade(context.Context) error
}

type AgentImageUpgrader interface {
	AgentUpgrader
	CreateGatewayClient(conn grpc.ClientConnInterface)
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

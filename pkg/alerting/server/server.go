package server

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/server/sync"
)

type InitializerF interface {
	InitOnce(f func())
	Initialized() bool
	WaitForInit()
	WaitForInitContext(ctx context.Context) error
}

type ServerComponent interface {
	InitializerF
	ComponentStatus
	Name() string

	SetConfig(config Config)
	// Server components that manage independent dependencies
	// should implement this method to sync them
	// Sync implementations should be cancellable on ctx.Err()
	Sync(ctx context.Context, info sync.SyncInfo) error
}

type Config struct {
	Client client.AlertingClient
}

type ComponentStatus interface {
	Ready() bool
	Healthy() bool
	Status() Status
}

type Status struct {
	Running bool
}

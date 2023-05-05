package update

import (
	"context"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PluginServer interface {
	SyncPluginManifest(context.Context, *controlv1.UpdateManifest) (*controlv1.SyncResults, error)
	GetPluginManifest(context.Context, *emptypb.Empty) (*controlv1.UpdateManifest, error)
}

type AgentServer interface {
	GetAgentManifest(context.Context, *emptypb.Empty) (*controlv1.UpdateManifest, error)
}

type UpdateServer struct {
	controlv1.UnsafeUpdateSyncServer
	PluginServer
	AgentServer
}

var _ controlv1.UpdateSyncServer = (*UpdateServer)(nil)

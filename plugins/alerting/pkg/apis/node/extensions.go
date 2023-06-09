package node

import (
	"context"

	"github.com/rancher/opni/pkg/agent/node"
	"google.golang.org/grpc"
)

var _ node.CapabilityConfig = (*AlertingCapabilityConfig)(nil)

func (a *AlertingCapabilityConfig) SetConditions(conds []string) {
	a.Conditions = conds
}

var _ node.AbstractSyncResponse[*AlertingCapabilityConfig] = (*SyncResponse)(nil)

type abstractAlertingSyncClient struct {
	client NodeAlertingCapabilityClient
}

func (a *abstractAlertingSyncClient) Sync(ctx context.Context, syncReq *AlertingCapabilityConfig, opt ...grpc.CallOption) (node.AbstractSyncResponse[*AlertingCapabilityConfig], error) {
	return a.client.Sync(ctx, syncReq, opt...)
}

func NewAbstractAlertingSyncClient(cc grpc.ClientConnInterface) node.AbstractNodeSyncClient[*AlertingCapabilityConfig] {
	client := NewNodeAlertingCapabilityClient(cc)
	return &abstractAlertingSyncClient{
		client: client,
	}
}

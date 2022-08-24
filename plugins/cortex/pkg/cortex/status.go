package cortex

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
)

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*cortexops.ClusterStatus, error) {
	return &cortexops.ClusterStatus{}, nil
}

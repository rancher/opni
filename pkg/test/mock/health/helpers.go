package mock_health

import (
	"context"
	"errors"

	"github.com/kralicky/gpkg/sync/atomic"
	"github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HealthStore struct {
	health              atomic.Value[*v1.Health]
	GetHealthShouldFail bool
}

func (hb *HealthStore) SetHealth(health *v1.Health) {
	health.Timestamp = timestamppb.Now()
	hb.health.Store(util.ProtoClone(health))
}

func (hb *HealthStore) GetHealth(context.Context, *emptypb.Empty, ...grpc.CallOption) (*v1.Health, error) {
	if hb.GetHealthShouldFail {
		return nil, errors.New("error")
	}
	return hb.health.Load(), nil
}

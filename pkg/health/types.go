package health

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type HealthClientSet interface {
	// rpc GetHealth(emptypb.Empty) returns (core.Health)
	GetHealth(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*corev1.Health, error)
}

type BackendHealthClientSet interface {
	// rpc GetHealth(emptypb.Empty) returns (core.BackendHealth)
	GetHealth(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*corev1.BackendHealth, error)
}

type HealthStatusUpdater interface {
	StatusC() chan StatusUpdate
	HealthC() chan HealthUpdate
}

type HealthStatusQuerier interface {
	GetHealthStatus(id string) *corev1.HealthStatus
	WatchHealthStatus(ctx context.Context) <-chan *corev1.ClusterHealthStatus
}

type StatusUpdate struct {
	ID     string         `json:"id"`
	Status *corev1.Status `json:"status"`
}

type HealthUpdate struct {
	ID     string         `json:"id"`
	Health *corev1.Health `json:"health"`
}

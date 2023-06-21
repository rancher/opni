package gateway

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type pinger struct {
	corev1.UnsafePingerServer
}

func (p *pinger) Ping(_ context.Context, _ *emptypb.Empty) (*corev1.PingResponse, error) {
	return &corev1.PingResponse{
		Message: "pong",
	}, nil
}

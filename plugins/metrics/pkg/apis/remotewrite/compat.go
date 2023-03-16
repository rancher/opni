package remotewrite

import (
	context "context"

	"github.com/prometheus/prometheus/prompb"
	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type PrometheusRemoteWriteClient interface {
	Push(ctx context.Context, in *prompb.WriteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type prometheusRemoteWriteClient struct {
	cc grpc.ClientConnInterface
}

// Push implements PrometheusRemoteWriteClient
func (c prometheusRemoteWriteClient) Push(ctx context.Context, in *prompb.WriteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/remotewrite.RemoteWrite/Push", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func AsPrometheusRemoteWriteClient(rwc RemoteWriteClient) PrometheusRemoteWriteClient {
	return prometheusRemoteWriteClient{cc: rwc.(*remoteWriteClient).cc}
}

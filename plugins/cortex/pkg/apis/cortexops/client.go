package cortexops

import (
	"context"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type OpsClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type OpsClientOption func(*OpsClientOptions)

func (o *OpsClientOptions) apply(opts ...OpsClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddress(addr string) OpsClientOption {
	return func(o *OpsClientOptions) {
		o.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) OpsClientOption {
	return func(o *OpsClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewClient(ctx context.Context, opts ...OpsClientOption) (CortexOpsClient, error) {
	options := OpsClientOptions{
		listenAddr: managementv1.DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		},
	}
	options.apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	return NewCortexOpsClient(cc), nil
}

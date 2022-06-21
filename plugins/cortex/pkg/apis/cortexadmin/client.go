package cortexadmin

import (
	"context"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AdminClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type AdminClientOption func(*AdminClientOptions)

func (o *AdminClientOptions) apply(opts ...AdminClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddress(addr string) AdminClientOption {
	return func(o *AdminClientOptions) {
		o.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) AdminClientOption {
	return func(o *AdminClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewClient(ctx context.Context, opts ...AdminClientOption) (CortexAdminClient, error) {
	options := AdminClientOptions{
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
	return NewCortexAdminClient(cc), nil
}

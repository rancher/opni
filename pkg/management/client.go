package management

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ManagementClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type ManagementClientOption func(*ManagementClientOptions)

func (o *ManagementClientOptions) apply(opts ...ManagementClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddress(addr string) ManagementClientOption {
	return func(o *ManagementClientOptions) {
		o.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) ManagementClientOption {
	return func(o *ManagementClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewClient(ctx waitctx.RestrictiveContext, opts ...ManagementClientOption) (managementv1.ManagementClient, error) {
	options := ManagementClientOptions{
		listenAddr: managementv1.DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		},
	}
	options.apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	waitctx.Go(ctx, func() {
		<-ctx.Done()
		cc.Close()
	})
	return managementv1.NewManagementClient(cc), nil
}

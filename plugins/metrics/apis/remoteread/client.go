package remoteread

import (
	"fmt"

	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type ClientOption func(*ClientOptions)

func (opt *ClientOptions) apply(opts ...ClientOption) {
	for _, op := range opts {
		op(opt)
	}
}

func WithListenAddress(addr string) ClientOption {
	return func(opt *ClientOptions) {
		opt.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) ClientOption {
	return func(opt *ClientOptions) {
		opt.dialOptions = append(opt.dialOptions, options...)
	}
}

func NewGatewayClient(ctx waitctx.PermissiveContext, opts ...ClientOption) (RemoteReadGatewayClient, error) {
	options := ClientOptions{
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		},
	}

	options.apply(opts...)

	connection, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("could not establish connection: %w", err)
	}

	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		connection.Close()
	})

	return NewRemoteReadGatewayClient(connection), nil
}

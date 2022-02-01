package management

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ManagementClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type ManagementClientOption func(*ManagementClientOptions)

func (o *ManagementClientOptions) Apply(opts ...ManagementClientOption) {
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

func NewClient(ctx context.Context, opts ...ManagementClientOption) (ManagementClient, error) {
	options := ManagementClientOptions{
		listenAddr: DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	}
	options.Apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	return NewManagementClient(cc), nil
}

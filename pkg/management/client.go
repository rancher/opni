package management

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ManagementClientOptions struct {
	listenAddr string
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

func NewClient(ctx context.Context, opts ...ManagementClientOption) (ManagementClient, error) {
	options := ManagementClientOptions{
		listenAddr: DefaultManagementSocket(),
	}
	options.Apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return NewManagementClient(cc), nil
}

package management

import (
	"google.golang.org/grpc"
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

func NewClient(opts ...ManagementClientOption) (ManagementClient, error) {
	options := ManagementClientOptions{
		listenAddr: DefaultManagementSocket(),
	}
	options.Apply(opts...)
	cc, err := grpc.Dial(options.listenAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return NewManagementClient(cc), nil
}

package management

import (
	"google.golang.org/grpc"
)

type ManagementClientOptions struct {
	socket string
}

type ManagementClientOption func(*ManagementClientOptions)

func (o *ManagementClientOptions) Apply(opts ...ManagementClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithSocket(socket string) ManagementClientOption {
	return func(o *ManagementClientOptions) {
		o.socket = socket
	}
}

func NewClient(opts ...ManagementClientOption) (ManagementClient, error) {
	options := ManagementClientOptions{
		socket: DefaultManagementSocket,
	}
	options.Apply(opts...)
	cc, err := grpc.Dial("unix://"+options.socket, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return NewManagementClient(cc), nil
}

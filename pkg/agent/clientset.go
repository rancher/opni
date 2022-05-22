package agent

import grpc "google.golang.org/grpc"

type ClientSet interface {
	RemoteControlClient
}

type clientSet struct {
	RemoteControlClient
}

func NewClientSet(cc grpc.ClientConnInterface) ClientSet {
	return &clientSet{
		RemoteControlClient: NewRemoteControlClient(cc),
	}
}

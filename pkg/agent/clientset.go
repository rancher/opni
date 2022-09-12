package agent

import (
	"google.golang.org/grpc"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type ClientSet interface {
	controlv1.HealthClient
}

type clientSet struct {
	controlv1.HealthClient
}

func NewClientSet(cc grpc.ClientConnInterface) ClientSet {
	return &clientSet{
		HealthClient: controlv1.NewHealthClient(cc),
	}
}

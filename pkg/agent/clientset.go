package agent

import (
	"google.golang.org/grpc"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type ClientSet interface {
	controlv1.HealthClient
	capabilityv1.NodeClient
}

type clientSet struct {
	controlv1.HealthClient
	capabilityv1.NodeClient
}

func NewClientSet(cc grpc.ClientConnInterface) ClientSet {
	return &clientSet{
		HealthClient: controlv1.NewHealthClient(cc),
		NodeClient:   capabilityv1.NewNodeClient(cc),
	}
}

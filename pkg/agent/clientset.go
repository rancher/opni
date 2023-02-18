package agent

import (
	"google.golang.org/grpc"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type ClientSet interface {
	controlv1.HealthClient
	capabilityv1.NodeClient
	ClientConn() grpc.ClientConnInterface
}

type clientSet struct {
	cc grpc.ClientConnInterface
	controlv1.HealthClient
	capabilityv1.NodeClient
}

func (c *clientSet) ClientConn() grpc.ClientConnInterface {
	return c.cc
}

func NewClientSet(cc grpc.ClientConnInterface) ClientSet {
	return &clientSet{
		cc:           cc,
		HealthClient: controlv1.NewHealthClient(cc),
		NodeClient:   capabilityv1.NewNodeClient(cc),
	}
}

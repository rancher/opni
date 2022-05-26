package agent

import (
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/grpc"
)

type ClientSet interface {
	controlv1.AgentControlClient
}

type clientSet struct {
	controlv1.AgentControlClient
}

func NewClientSet(cc grpc.ClientConnInterface) ClientSet {
	return &clientSet{
		AgentControlClient: controlv1.NewAgentControlClient(cc),
	}
}

package gateway

import (
	"context"

	"github.com/rancher/opni/pkg/agent"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AgentMonitor struct {
	logger *zap.SugaredLogger
}

func (g *AgentMonitor) HandleAgentConnection(ctx context.Context, clientset agent.ClientSet) {
	g.logger.Info("Agent connected")
	defer g.logger.Info("Agent disconnected")
	health, err := clientset.GetHealth(ctx, &emptypb.Empty{})
	if err != nil {
		g.logger.Error(err)
	} else {
		g.logger.Info("Agent health: " + health.String())
	}
	<-ctx.Done()
}

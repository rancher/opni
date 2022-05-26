package gateway

import (
	"context"
	"time"

	"github.com/lthibault/jitterbug/v2"
	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/auth/cluster"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AgentMonitor struct {
	logger *zap.SugaredLogger
}

func (g *AgentMonitor) HandleAgentConnection(ctx context.Context, clientset agent.ClientSet) {
	id := cluster.StreamAuthorizedID(ctx)
	lg := g.logger.With("id", id)
	lg.Info("Agent connected")
	defer lg.Info("Agent disconnected")
	ticker := jitterbug.New(5*time.Second, jitterbug.Uniform{
		Min: 1 * time.Second,
	})
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			health, err := clientset.GetHealth(ctx, &emptypb.Empty{})
			if err != nil {
				lg.Error(err)
			} else {
				lg.Debug("Agent health: " + health.String())
			}
		}
	}
}

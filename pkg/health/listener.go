package health

import (
	"context"
	"time"

	"github.com/lthibault/jitterbug/v2"
	"github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Listener struct {
	statusUpdate chan StatusUpdate
	healthUpdate chan HealthUpdate
}

func NewListener() *Listener {
	return &Listener{
		statusUpdate: make(chan StatusUpdate, 100),
		healthUpdate: make(chan HealthUpdate, 100),
	}
}

func (l *Listener) HandleConnection(ctx context.Context, clientset HealthClientSet) {
	id := cluster.StreamAuthorizedID(ctx)
	l.statusUpdate <- StatusUpdate{
		ID: id,
		Status: &corev1.Status{
			Connected: true,
		},
	}
	defer func() {
		l.statusUpdate <- StatusUpdate{
			ID: id,
			Status: &corev1.Status{
				Connected: false,
			},
		}
	}()
	curHealth, err := clientset.GetHealth(ctx, &emptypb.Empty{})
	if err == nil {
		l.healthUpdate <- HealthUpdate{
			ID:     id,
			Health: util.ProtoClone(curHealth),
		}
	}

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
			if err == nil {
				if !proto.Equal(health, curHealth) {
					curHealth = health
					l.healthUpdate <- HealthUpdate{
						ID:     id,
						Health: util.ProtoClone(curHealth),
					}
				}
			}
		}
	}
}

func (l *Listener) StatusC() chan StatusUpdate {
	return l.statusUpdate
}

func (l *Listener) HealthC() chan HealthUpdate {
	return l.healthUpdate
}

// Implements gateway.ConnectionHandler
func (l *Listener) HandleAgentConnection(ctx context.Context, clientset agent.ClientSet) {
	l.HandleConnection(ctx, clientset)
}

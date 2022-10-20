package health

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	natsutil "github.com/rancher/opni/pkg/util/nats"
)

func jetstreamCtx(ctx context.Context) (nats.JetStreamContext, error) {
	nc, err := natsutil.AcquireNATSConnection(ctx)
	if err != nil {
		return nil, err
	}
	mgr, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	shared.NewAlertingDisconnectStream(mgr)
	return mgr, nil
}

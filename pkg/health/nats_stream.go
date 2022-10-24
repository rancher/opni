package health

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	natsutil "github.com/rancher/opni/pkg/util/nats"
)

func jetstreamCtx(ctx context.Context) (nats.JetStreamContext, error) {
	nc, err := natsutil.AcquireNATSConnection(
		ctx,
		natsutil.WithCreateStreams(shared.NewAlertingDisconnectStream()))
	if err != nil {
		return nil, err
	}
	mgr, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

package nats

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"
)

type natsAcquireOptions struct {
	lg       *zap.SugaredLogger
	retrier  backoffv2.Policy
	natsOpts []nats.Option
	streams  []*nats.StreamConfig
}

func (o *natsAcquireOptions) apply(opts ...NatsAcquireOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type NatsAcquireOption func(*natsAcquireOptions)

func WithNatsOptions(opts []nats.Option) NatsAcquireOption {
	return func(o *natsAcquireOptions) {
		o.natsOpts = opts
	}
}

func WithLogger(lg *zap.SugaredLogger) NatsAcquireOption {
	return func(o *natsAcquireOptions) {
		o.lg = lg
	}
}

func WithRetrier(retrier backoffv2.Policy) NatsAcquireOption {
	return func(o *natsAcquireOptions) {
		o.retrier = retrier
	}
}

func WithCreateStreams(streamNames ...*nats.StreamConfig) NatsAcquireOption {
	return func(o *natsAcquireOptions) {
		o.streams = streamNames
	}
}

// !!blocking
//
// default retrier policy :
//
// retrier := backoffv2.Exponential(
//
//	backoffv2.WithMaxRetries(0),
//	backoffv2.WithMinInterval(5*time.Second),
//	backoffv2.WithMaxInterval(1*time.Minute),
//	backoffv2.WithMultiplier(1.1),
//
// )
//
// default nats client options:
//
// SeedKey : os.Getenv("NKEY_SEED_FILENAME")
//
// nats.MaxReconnects(-1),
// nats.CustomReconnectDelay(
//
//	func(i int) time.Duration {
//		if i == 1 {
//			retryBackoff.Reset()
//		}
//		return retryBackoff.NextBackOff()
//	},
//
// ),
// nats.DisconnectErrHandler(
//
//	func(nc *nats.Conn, err error) {
//		lg.Error(err)
//	},
//
// ),
func AcquireNATSConnection(ctx context.Context, opts ...NatsAcquireOption) (*nats.Conn, error) {
	options := &natsAcquireOptions{
		lg: logger.NewPluginLogger().Named("nats-conn"),
		retrier: backoffv2.Exponential(
			backoffv2.WithMaxRetries(0),
			backoffv2.WithMinInterval(5*time.Second),
			backoffv2.WithMaxInterval(1*time.Minute),
			backoffv2.WithMultiplier(1.1),
		),
	}
	options.apply(opts...)
	var (
		nc  *nats.Conn
		err error
	)

	b := options.retrier.Start(ctx)
	for backoffv2.Continue(b) {
		nc, err = newNatsConnection(options.lg)
		if err == nil {
			break
		}
		options.lg.With("error", err).Warn("failed to connect to nats server, retrying")
	}
	mgr, err := nc.JetStream()
	if err == nil {
		for _, stream := range options.streams {
			err = NewPersistentStream(mgr, stream)
			if err != nil {
				options.lg.Error(err)
			}
		}
	} else {
		options.lg.Error(err)
	}

	return nc, err
}

// until we have a better way to do this
func newNatsConnection(lg *zap.SugaredLogger, options ...nats.Option) (*nats.Conn, error) {
	natsURL := os.Getenv("NATS_SERVER_URL")
	natsSeedPath := os.Getenv("NKEY_SEED_FILENAME")

	opt, err := nats.NkeyOptionFromSeed(natsSeedPath)
	if err != nil {
		return nil, err
	}
	retryBackoff := backoff.NewExponentialBackOff()
	defaultOps := []nats.Option{
		opt,
		nats.MaxReconnects(-1),
		nats.CustomReconnectDelay(
			func(i int) time.Duration {
				if i == 1 {
					retryBackoff.Reset()
				}
				return retryBackoff.NextBackOff()
			},
		),
		nats.DisconnectErrHandler(
			func(nc *nats.Conn, err error) {
				lg.Error(err)
			},
		),
	}
	defaultOps = append(defaultOps, options...)

	return nats.Connect(
		natsURL,
		defaultOps...,
	)
}

func NewPersistentStream(mgr nats.JetStreamContext, streamConfig *nats.StreamConfig) error {
	if stream, _ := mgr.StreamInfo(streamConfig.Name); stream == nil {
		_, err := mgr.AddStream(streamConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewDurableReplayConsumer(mgr nats.JetStreamContext, streamName string, consumerConfig *nats.ConsumerConfig) error {
	if consumerConfig.Durable == "" {
		return fmt.Errorf("consumer config must be durable")
	}
	if consumerConfig.ReplayPolicy != nats.ReplayInstantPolicy {
		return fmt.Errorf("consumer config must be replay instant policy")
	}
	if _, err := mgr.ConsumerInfo(streamName, consumerConfig.Durable); err != nil {
		_, err := mgr.AddConsumer(streamName, consumerConfig)
		if err != nats.ErrConsumerNameAlreadyInUse {
			return err
		}
	}
	return nil
}

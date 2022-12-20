package alerting

import (
	"context"
	"encoding/json"
	"os"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"go.uber.org/zap"
)

type WatcherHooks[T any] interface {
	RegisterEvent(eventType func(T) bool, hooks ...func(context.Context, T) error)
	HandleEvent(T)
}

// Handles admitting hooks
// WIP
type AdmissionWatcher[T any, Client any] interface {
	WatcherHooks[T]
	AcquireClient() func() (Client, error)
	Heartbeat(Client) (T, error)
}

// WIP
func RunAdmission[T any, Client any](ctx context.Context, lg *zap.SugaredLogger, watcher AdmissionWatcher[T, Client]) {
	c, err := watcher.AcquireClient()()
	if err != nil {
		lg.Errorf("failed to acquire client: %v", err)
		os.Exit(1)
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			event, err := watcher.Heartbeat(c)
			if err != nil {
				lg.Errorf("failed to receive information: %v", err)
				continue
			}
			watcher.HandleEvent(event)
		}
	}
}

// WIP
type IngesterWatcher[T any, Client any] interface {
	RequiredStream() *nats.StreamConfig
	AcquireClient() func() (Client, error)
	Before() func(Client) error
	Heartbeat(Client) (T, error)
	Sanitize(T) bool
	StreamSubject() func(T) string
	After() func(Client)
}

// WIP
func RunIngester[T any, Client any](
	ctx context.Context,
	lg *zap.SugaredLogger,
	js nats.JetStreamContext,
	watcher IngesterWatcher[T, Client]) {
	if watcher.RequiredStream() == nil {
		panic("stream config is required")
	}
	stream := watcher.RequiredStream()
	if err := natsutil.NewPersistentStream(js, stream); err != nil {
		lg.Errorf("failed to create stream: %v", err)
		os.Exit(1)
	}
	var c Client
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(10),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(60*time.Second),
		backoffv2.WithMultiplier(1.2),
	)
	b := retrier.Start(ctx)
	for backoffv2.Continue(b) {
		if client, err := watcher.AcquireClient()(); err != nil {
			lg.Warnf("failed to acquire client: %v", err)
		} else {
			c = client
			break
		}
	}
	defer watcher.After()(c)
	if watcher.Before() != nil {
		err := watcher.Before()(c)
		if err != nil {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			lg.Info("exiting from ingester")
			return
		default:
			if heartbeat, err := watcher.Heartbeat(c); err != nil {
				lg.Errorf("failed to get heartbeat: %v", err)
			} else {
				if watcher.Sanitize(heartbeat) {
					subject := watcher.StreamSubject()(heartbeat)
					go func() {
						data, err := json.Marshal(heartbeat)
						if err != nil {
							lg.Errorf("failed to marshal heartbeat: %v", err)
							return
						}
						_, err = js.PublishAsync(subject, data)
						if err != nil {
							lg.Errorf("failed to publish heartbeat: %v", err)
						}
					}()

				}
			}
		}
	}
}

type InternalConditionWatcher interface {
	WatchEvents()
}

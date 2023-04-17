package otel

import (
	"context"
	"sync"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util/future"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ClientProvider struct {
	ctx context.Context

	setMu    *sync.Mutex
	targetMu *sync.RWMutex

	setAddressCtx       context.Context
	setAdressCancelFunc context.CancelFunc
	remoteTarget        future.Future[colmetricspb.MetricsServiceClient]

	otelInitializerOptions
}

func NewClientProvider(ctx context.Context, opts ...OTELInitializerOption) ClientProvider {
	options := &otelInitializerOptions{
		logger: logger.NewPluginLogger().Named("metrics-otel-intializer"),
	}
	options.apply(opts...)
	c := ClientProvider{
		ctx:                    ctx,
		setMu:                  &sync.Mutex{},
		targetMu:               &sync.RWMutex{},
		remoteTarget:           future.New[colmetricspb.MetricsServiceClient](),
		otelInitializerOptions: *options,
	}
	if options.remoteAddress != "" {
		go c.SetAddress(options.remoteAddress)
	}
	return c
}

func (c *ClientProvider) GetRemoteTarget() (colmetricspb.MetricsServiceClient, error) {
	c.targetMu.RLock()
	defer c.targetMu.RUnlock()
	if !c.remoteTarget.IsSet() {
		return nil, status.Error(codes.Unavailable, "remote OTLP target not available")
	}
	return c.remoteTarget.Get(), nil
}

func (c *ClientProvider) HasRemoteTarget() bool {
	c.targetMu.RLock()
	defer c.targetMu.RUnlock()
	return c.remoteTarget.IsSet()
}

func (c *ClientProvider) SetClient(client colmetricspb.MetricsServiceClient) {
	if c.remoteTarget.IsSet() {
		c.remoteTarget = future.New[colmetricspb.MetricsServiceClient]()
	}
	c.remoteTarget.Set(client)
}

func (c *ClientProvider) SetAddress(address string) {
	c.setMu.Lock()
	if c.setAddressCtx != nil {
		c.setAdressCancelFunc()
	}
	// capture in goroutine
	setAddressCtx, setAddressCancelFunc := context.WithCancel(c.ctx)
	c.setAddressCtx, c.setAdressCancelFunc = setAddressCtx, setAddressCancelFunc
	c.setMu.Unlock()

	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(3*time.Second),
		backoff.WithMaxInterval(15*time.Second),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(c.setAddressCtx)
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-setAddressCtx.Done():
				return
			case <-b.Next():
				if address == "" { // skip acquiring a new client if unset
					continue
				}
				conn, err := grpc.Dial(
					address,
					c.targetDialOptions...,
				)
				if err != nil {
					c.logger.Warnf("failed to dial to remote target : %s", err)
					continue
				}
				c.targetMu.Lock()
				defer c.targetMu.Unlock()
				if c.remoteTarget.IsSet() {
					c.remoteTarget = future.New[colmetricspb.MetricsServiceClient]()
				}
				c.remoteTarget.Set(colmetricspb.NewMetricsServiceClient(conn))
				return
			}
		}
	}()
}

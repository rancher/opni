package storage_opts

import (
	"time"

	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"go.uber.org/zap"
)

type RequestOptions struct {
	Unredacted bool
}

type RequestOption func(*RequestOptions)

func WithUnredacted() RequestOption {
	return func(o *RequestOptions) {
		o.Unredacted = true
	}
}

func (o *RequestOptions) Apply(opts ...RequestOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type SyncOptions struct {
	Router  routing.OpniRouting
	Timeout time.Duration
}

type SyncOption func(*SyncOptions)

func WithSyncTimeout(timeout time.Duration) SyncOption {
	return func(o *SyncOptions) {
		o.Timeout = timeout
	}
}

func WithInitialRouter(router routing.OpniRouting) SyncOption {
	return func(o *SyncOptions) {
		o.Router = router
	}
}

func (o *SyncOptions) Apply(opts ...SyncOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func NewSyncOptions() *SyncOptions {
	return &SyncOptions{
		Timeout: 2 * time.Minute,
		Router:  routing.NewDefaultOpniRouting(),
	}
}

type ClientSetOptions struct {
	Logger     *zap.SugaredLogger
	Timeout    time.Duration
	TrackerTTl time.Duration
}

type ClientSetOption func(*ClientSetOptions)

func WithStorageTimeout(timeout time.Duration) ClientSetOption {
	return func(o *ClientSetOptions) {
		o.Timeout = timeout
	}
}

func WithLogger(lg *zap.SugaredLogger) ClientSetOption {
	return func(o *ClientSetOptions) {
		o.Logger = lg
	}
}

func (s *ClientSetOptions) Apply(opts ...ClientSetOption) {
	for _, opt := range opts {
		opt(s)
	}
}

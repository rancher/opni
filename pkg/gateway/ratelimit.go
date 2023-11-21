package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	tokenbucket "golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RatelimiterInterceptor struct {
	conf        reactive.Reactive[*configv1.RateLimitingSpec]
	tokenBucket atomic.Pointer[tokenbucket.Limiter]
	lg          *slog.Logger
}

func NewRateLimiterInterceptor(ctx context.Context, lg *slog.Logger, conf reactive.Reactive[*configv1.RateLimitingSpec]) *RatelimiterInterceptor {
	r := &RatelimiterInterceptor{
		lg:   lg,
		conf: conf,
	}
	conf.WatchFunc(ctx, func(value *configv1.RateLimitingSpec) {
		r.lg.With("rate", value.GetRate(), "burst", value.GetBurst()).Info("updating ratelimit parameters")
		r.tokenBucket.Store(tokenbucket.NewLimiter(tokenbucket.Limit(value.GetRate()), int(value.GetBurst())))
	})
	return r
}

func (r *RatelimiterInterceptor) allow() bool {
	bucket := r.tokenBucket.Load()
	if bucket == nil {
		return true
	}
	r.lg.Debug(fmt.Sprintf("ratelimit: %f available", bucket.Tokens()))
	return bucket.Allow()
}

func (r *RatelimiterInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if r.allow() {
			return handler(srv, stream)
		}
		return status.Errorf(codes.ResourceExhausted, "%s is unable to handle the request, please retry later.", info.FullMethod)
	}
}

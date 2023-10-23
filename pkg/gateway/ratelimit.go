package gateway

import (
	"fmt"
	"log/slog"

	tokenbucket "golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RatelimiterInterceptor struct {
	tokenBucket *tokenbucket.Limiter
	lg          *slog.Logger
}

type ratelimiterOptions struct {
	rate  float64
	burst int
}

type RatelimiterOption func(*ratelimiterOptions)

func (o *ratelimiterOptions) apply(opts ...RatelimiterOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func WithRate(rate float64) RatelimiterOption {
	return func(o *ratelimiterOptions) {
		o.rate = rate
	}
}

func WithBurst(burst int) RatelimiterOption {
	return func(o *ratelimiterOptions) {
		o.burst = burst
	}
}

func NewRateLimiterInterceptor(lg *slog.Logger, opts ...RatelimiterOption) *RatelimiterInterceptor {
	options := &ratelimiterOptions{
		rate:  10,
		burst: 50,
	}
	options.apply(opts...)

	return &RatelimiterInterceptor{
		lg: lg,
		tokenBucket: tokenbucket.NewLimiter(
			tokenbucket.Limit(options.rate),
			options.burst,
		),
	}
}

func (r *RatelimiterInterceptor) allow() bool {
	r.lg.Debug(fmt.Sprintf("ratelimit: %f available", r.tokenBucket.Tokens()))
	return r.tokenBucket.Allow()
}

func (r *RatelimiterInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if r.allow() {
			return handler(srv, stream)
		}
		return status.Errorf(codes.ResourceExhausted, "%s is unable to handle the request, please retry later.", info.FullMethod)
	}
}

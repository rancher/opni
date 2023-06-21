package gateway

import (
	"go.uber.org/zap"
	tokenbucket "golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ratelimiterInterceptor struct {
	tokenBucket *tokenbucket.Limiter
	lg          *zap.SugaredLogger
}

type ratelimiterOptions struct {
	rate  float64
	burst int
}

type ratelimiterOption func(*ratelimiterOptions)

func (o *ratelimiterOptions) apply(opts ...ratelimiterOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func WithRate(rate float64) ratelimiterOption {
	return func(o *ratelimiterOptions) {
		o.rate = rate
	}
}

func WithBurst(burst int) ratelimiterOption {
	return func(o *ratelimiterOptions) {
		o.burst = burst
	}
}

func NewRateLimiterInterceptor(lg *zap.SugaredLogger, opts ...ratelimiterOption) *ratelimiterInterceptor {
	options := &ratelimiterOptions{
		rate:  10,
		burst: 50,
	}
	options.apply(opts...)

	return &ratelimiterInterceptor{
		lg: lg,
		tokenBucket: tokenbucket.NewLimiter(
			tokenbucket.Limit(options.rate),
			options.burst,
		),
	}
}

func (r *ratelimiterInterceptor) allow() bool {
	r.lg.Debugf("ratelimit: %d available", r.tokenBucket.Tokens())
	return r.tokenBucket.Allow()
}

func (r *ratelimiterInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if r.allow() {
			return handler(srv, stream)
		}
		return status.Errorf(codes.ResourceExhausted, "%s is unable to handle the request, please retry later.", info.FullMethod)
	}
}

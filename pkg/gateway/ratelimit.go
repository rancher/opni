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

func NewRateLimiterInterceptor(lg *zap.SugaredLogger) *ratelimiterInterceptor {
	return &ratelimiterInterceptor{
		lg:          lg,
		tokenBucket: tokenbucket.NewLimiter(10, 50),
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

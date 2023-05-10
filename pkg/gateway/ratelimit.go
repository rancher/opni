package gateway

import (
	tokenbucket "github.com/juju/ratelimit"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ratelimiterInterceptor struct {
	tokenBucket *tokenbucket.Bucket
	lg          *zap.SugaredLogger
}

func NewRateLimiterInterceptor(lg *zap.SugaredLogger) *ratelimiterInterceptor {
	return &ratelimiterInterceptor{
		lg:          lg,
		tokenBucket: tokenbucket.NewBucketWithRate(10, 50),
	}
}

func (r *ratelimiterInterceptor) limit() bool {
	r.lg.Debugf("ratelimit: %d available", r.tokenBucket.Available())
	return r.tokenBucket.TakeAvailable(1) == 0
}

func (r *ratelimiterInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if r.limit() {
			return status.Errorf(codes.ResourceExhausted, "%s is unable to handle the request, please retry later.", info.FullMethod)
		}
		return handler(srv, stream)
	}
}

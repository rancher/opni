package health

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type ServiceHealthChecker struct {
	// The grpc health check method name to use
	HealthCheckMethod string
	// The service to check
	Service string
}

type ServingStatus struct {
	v atomic.Int32
}

func (s *ServingStatus) Set(status healthpb.HealthCheckResponse_ServingStatus) {
	s.v.Store(int32(status))
}

func (s *ServingStatus) Get() healthpb.HealthCheckResponse_ServingStatus {
	return healthpb.HealthCheckResponse_ServingStatus(s.v.Load())
}

func (s *ServiceHealthChecker) Start(ctx context.Context, cc *grpc.ClientConn, callback func(healthpb.HealthCheckResponse_ServingStatus)) error {
	stream, err := cc.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
	}, s.HealthCheckMethod)
	if err != nil {
		return err
	}

	err = stream.SendMsg(&healthpb.HealthCheckRequest{Service: s.Service})
	if err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}

	go func() {
		for {
			resp := new(healthpb.HealthCheckResponse)
			err = stream.RecvMsg(resp)
			if err != nil {
				if errors.Is(err, io.EOF) {
					callback(healthpb.HealthCheckResponse_NOT_SERVING)
					return
				}

				if status.Code(err) == codes.Unimplemented {
					callback(healthpb.HealthCheckResponse_SERVICE_UNKNOWN)
				} else {
					callback(healthpb.HealthCheckResponse_NOT_SERVING)
				}
				return
			}

			callback(resp.Status)
		}
	}()

	return nil
}

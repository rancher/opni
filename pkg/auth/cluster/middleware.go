package cluster

import (
	"context"

	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/util/streams"
	"google.golang.org/grpc"
)

func StreamServerInterceptor(challenge challenges.ChallengeHandler) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		stream := streams.NewServerStreamWithContext(ss)
		stream.Ctx = challenge.InterceptContext(stream.Ctx)
		var err error
		if stream.Ctx, err = challenge.DoChallenge(stream); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func StreamClientInterceptor(challenge challenges.ChallengeHandler) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		cs, err := streamer(challenge.InterceptContext(ctx), desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		stream := streams.NewClientStreamWithContext(cs)
		if stream.Ctx, err = challenge.DoChallenge(stream); err != nil {
			return nil, err
		}
		return stream, err
	}
}

package mock_auth

import (
	"context"

	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util/streams"
	"google.golang.org/grpc/metadata"
)

func ContextWithAuthorizedID(ctx context.Context, clusterID string) context.Context {
	return context.WithValue(ctx, cluster.ClusterIDKey, clusterID)
}

type TestChallengeHandler struct {
	clientChallenge challenges.HandlerFunc
	md              []string
}

var _ challenges.ChallengeHandler = &TestChallengeHandler{}

func (t *TestChallengeHandler) DoChallenge(ss streams.Stream) (context.Context, error) {
	return t.clientChallenge(ss)
}

func (t *TestChallengeHandler) InterceptContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, t.md...)
}

func NewTestChallengeHandler(fn challenges.HandlerFunc, kvs ...string) *TestChallengeHandler {
	return &TestChallengeHandler{
		clientChallenge: fn,
		md:              kvs,
	}
}

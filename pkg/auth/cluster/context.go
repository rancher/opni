package cluster

import (
	"context"

	"github.com/rancher/opni/pkg/keyring"
	"google.golang.org/grpc/metadata"
)

type (
	clusterIDKeyType  string
	sharedKeysKeyType string
)

const (
	ClusterIDKey  clusterIDKeyType  = "cluster_auth_cluster_id"
	SharedKeysKey sharedKeysKeyType = "cluster_auth_shared_keys"
)

type ClientMetadata struct {
	IdAssertion string
	Random      []byte
}

func StreamAuthorizedKeys(ctx context.Context) *keyring.SharedKeys {
	return ctx.Value(SharedKeysKey).(*keyring.SharedKeys)
}

func StreamAuthorizedID(ctx context.Context) string {
	return ctx.Value(ClusterIDKey).(string)
}

func AuthorizedOutgoingContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, string(ClusterIDKey), StreamAuthorizedID(ctx))
}

func AuthorizedIDFromIncomingContext(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	ids := md.Get(string(ClusterIDKey))
	if len(ids) == 0 {
		return "", false
	}
	return ids[0], true
}

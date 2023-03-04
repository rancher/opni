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

func (k clusterIDKeyType) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if v := ctx.Value(k); v != nil {
		return map[string]string{
			string(k): v.(string),
		}, nil
	}
	return nil, nil
}

func (k clusterIDKeyType) RequireTransportSecurity() bool {
	return false
}

func (k clusterIDKeyType) FromIncomingCredentials(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	if values := md.Get(string(k)); len(values) == 1 {
		ctx = context.WithValue(ctx, k, values[0])
	}
	return ctx
}

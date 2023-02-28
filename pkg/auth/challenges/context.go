package challenges

import (
	"context"
	"encoding/base64"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (

	// Deprecated: only used in challenge v1
	ChallengeKey = "X-Challenge"

	ChallengeVersionMetadataKey  = "x-challenge-version"
	ClientIdAssertionMetadataKey = "x-client-id-assertion"
	ClientRandomMetadataKey      = "x-client-random"

	ChallengeV2 = "v2"
)

type ClientMetadata struct {
	IdAssertion string
	Random      []byte
}

func ClientIdAssertionFromMetadata(md metadata.MD) (string, bool) {
	ids := md.Get(ClientIdAssertionMetadataKey)
	if len(ids) == 0 || len(ids[0]) == 0 {
		return "", false
	}
	return ids[0], true
}

func ClientRandomFromMetadata(md metadata.MD) ([]byte, bool) {
	v := md.Get(ClientRandomMetadataKey)
	if len(v) == 0 || len(v[0]) == 0 {
		return nil, false
	}

	data, err := base64.RawURLEncoding.DecodeString(v[0])
	if err != nil {
		return nil, false
	}
	return data, true
}

func ClientMetadataFromIncomingContext(ctx context.Context) (ClientMetadata, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing metadata")
	}
	assertedId, ok := ClientIdAssertionFromMetadata(md)
	if !ok {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing id in metadata")
	}
	clientRandom, ok := ClientRandomFromMetadata(md)
	if !ok || len(clientRandom) == 0 {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing/invalid client random in metadata")
	}

	return ClientMetadata{
		IdAssertion: assertedId,
		Random:      clientRandom,
	}, nil
}

func ClientMetadataFromOutgoingContext(ctx context.Context) (ClientMetadata, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing metadata")
	}
	assertedId, ok := ClientIdAssertionFromMetadata(md)
	if !ok {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing id in metadata")
	}
	clientRandom, ok := ClientRandomFromMetadata(md)
	if !ok || len(clientRandom) == 0 {
		return ClientMetadata{}, status.Errorf(codes.InvalidArgument, "missing/invalid client random in metadata")
	}

	return ClientMetadata{
		IdAssertion: assertedId,
		Random:      clientRandom,
	}, nil
}

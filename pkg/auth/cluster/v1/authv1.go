package authv1

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"log/slog"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	DomainString = ""
)

type Server struct {
	method   string
	logger   *slog.Logger
	verifier challenges.KeyringVerifier
}

// Deprecated: authv1 is deprecated and will be removed in a future release.
func NewServerChallenge(method string, verifier challenges.KeyringVerifier, logger *slog.Logger) challenges.ChallengeHandler {
	return &Server{
		logger:   logger,
		method:   method,
		verifier: verifier,
	}
}

func (a *Server) InterceptContext(ctx context.Context) context.Context {
	return ctx
}

func (a *Server) DoChallenge(ss streams.Stream) (context.Context, error) {
	challenge := uuid.New()
	if err := ss.(grpc.ServerStream).SendHeader(metadata.Pairs(challenges.ChallengeKey, challenge.String())); err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to send challenge: %v", err)
	}
	method, ok := grpc.MethodFromServerStream(ss.(grpc.ServerStream))
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to get method name from stream")
	}

	challengeResponse := &corev1.ChallengeResponse{}
	select {
	case <-ss.Context().Done():
		return nil, ss.Context().Err()
	case <-time.After(1 * time.Second):
		return nil, status.Errorf(codes.DeadlineExceeded, "timed out waiting for challenge response")
	case err := <-lo.Async(func() error { return ss.RecvMsg(challengeResponse) }):
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "error receiving challenge response: %v", err)
		}
		if err := authutil.CheckUnknownFields(a.logger, challengeResponse); err != nil {
			return nil, err
		}
		if len(challengeResponse.GetResponse()) == 0 {
			return nil, status.Errorf(codes.InvalidArgument, "empty challenge response received")
		}
	}

	errVerifyFailed := util.StatusError(codes.Unauthenticated)

	id, nonce, mac, err := decodeLegacyAuthHeader(challengeResponse.Response)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid challenge response received: %v", err)
	}

	if subtle.ConstantTimeCompare(nonce, []byte(challenge.String())) != 1 {
		return nil, errVerifyFailed
	}

	simReq := &corev1.ChallengeRequest{
		Challenge: append(challenge[:], []byte(method)...),
	}

	simResp := corev1.ChallengeResponse{
		Response: mac,
	}

	cm := challenges.ClientMetadata{IdAssertion: string(id)}
	cachedVerifier, err := a.verifier.Prepare(ss.Context(), cm, simReq)
	if err != nil {
		return nil, err
	}
	sharedKeys := cachedVerifier.Verify(&simResp)
	if sharedKeys == nil {
		return nil, errVerifyFailed
	}

	ctx := context.WithValue(ss.Context(), cluster.SharedKeysKey, sharedKeys)
	ctx = context.WithValue(ctx, cluster.ClusterIDKey, cm.IdAssertion)

	return ctx, nil
}

func decodeLegacyAuthHeader(hdr []byte) (id, nonce, mac []byte, err error) {
	// Decode the legacy mac auth header format, which is going to be exactly:
	// MAC id="%s",nonce="%s",mac="%s"
	// where id is a base64-encoded v4 uuid, nonce is a v4 uuid, and mac is a base64 encoded 512-bit mac.

	// We can "hard-code" the decoding logic here, as it only exists in past versions of the code.

	if len(hdr) != 195 {
		err = fmt.Errorf("invalid auth header length")
		return
	}
	id, _ = base64.RawURLEncoding.DecodeString(string(hdr[8 : 8+48]))
	nonce = hdr[65 : 65+36]
	mac, _ = base64.RawURLEncoding.DecodeString(string(hdr[108 : 108+86]))
	return
}

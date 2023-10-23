package authv2

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"golang.org/x/crypto/blake2b"
	"log/slog"

	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	DomainString = "agent auth v2"
)

type Client struct {
	logger     *slog.Logger
	clientId   string
	sharedKeys *keyring.SharedKeys
}

func NewClientChallenge(kr keyring.Keyring, clientId string, logger *slog.Logger) (challenges.ChallengeHandler, error) {
	var sharedKeys *keyring.SharedKeys
	ok := kr.Try(func(keys *keyring.SharedKeys) {
		sharedKeys = keys
	})
	if !ok {
		return nil, status.Errorf(codes.DataLoss, "keyring is missing shared keys")
	}
	return &Client{
		logger:     logger,
		clientId:   clientId,
		sharedKeys: sharedKeys,
	}, nil
}

type Server struct {
	ChallengeOptions
	logger   *slog.Logger
	verifier challenges.KeyringVerifier
}

func (a *Server) InterceptContext(ctx context.Context) context.Context {
	return ctx
}

type ChallengeOptions struct {
	challengeTimeout time.Duration
}

type ChallengeOption func(*ChallengeOptions)

func (o *ChallengeOptions) apply(opts ...ChallengeOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithChallengeTimeout(challengeTimeout time.Duration) ChallengeOption {
	return func(o *ChallengeOptions) {
		o.challengeTimeout = challengeTimeout
	}
}

func NewServerChallenge(verifier challenges.KeyringVerifier, logger *slog.Logger, opts ...ChallengeOption) challenges.ChallengeHandler {
	options := ChallengeOptions{
		challengeTimeout: 1 * time.Second,
	}
	options.apply(opts...)
	return &Server{
		ChallengeOptions: options,
		logger:           logger,
		verifier:         verifier,
	}
}

func (a *Client) InterceptContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		challenges.ClientIdAssertionMetadataKey, a.clientId,
		challenges.ClientRandomMetadataKey, base64.RawURLEncoding.EncodeToString(authutil.NewRandom256()),
		challenges.ChallengeVersionMetadataKey, challenges.ChallengeV2,
	)
}

func (a *Server) DoChallenge(ss streams.Stream) (context.Context, error) {
	cm, err := challenges.ClientMetadataFromIncomingContext(ss.Context())
	if err != nil {
		return nil, err
	}

	challengeRequests := &corev1.ChallengeRequestList{
		Items: []*corev1.ChallengeRequest{
			{
				Challenge: authutil.NewRandom256(),
			},
		},
	}

	cachedVerifier, err := a.verifier.Prepare(ss.Context(), cm, challengeRequests.Items[0])
	if err != nil {
		return nil, err
	}

	if err := ss.SendMsg(challengeRequests); err != nil {
		return nil, status.Errorf(codes.Aborted, "error sending challenge: %v", err)
	}

	challengeResponses := &corev1.ChallengeResponseList{}

	select {
	case <-ss.Context().Done():
		return nil, ss.Context().Err()
	case <-time.After(a.challengeTimeout):
		return nil, status.Errorf(codes.DeadlineExceeded, "timed out waiting for challenge response")
	case err := <-lo.Async(func() error { return ss.RecvMsg(challengeResponses) }):
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "error receiving challenge response: %v", err)
		}
		if err := authutil.CheckUnknownFields(a.logger, challengeResponses); err != nil {
			return nil, err
		}
		if len(challengeResponses.Items) != 1 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid challenge response received")
		}
	}

	challengeResponse := challengeResponses.Items[0]

	errVerifyFailed := util.StatusError(codes.Unauthenticated)
	sharedKeys := cachedVerifier.Verify(challengeResponse)
	if sharedKeys == nil {
		return nil, errVerifyFailed
	}

	authInfo := &corev1.AuthInfo{
		AuthorizedId: cm.IdAssertion,
	}

	{
		// compute a mac over the metadata, challenge requests, and responses
		mac, _ := blake2b.New512(sharedKeys.ServerKey)
		mac.Write([]byte(cm.IdAssertion))
		mac.Write(cm.Random)
		for _, cr := range challengeRequests.Items {
			mac.Write(cr.Challenge)
		}
		for _, cr := range challengeResponses.Items {
			mac.Write(cr.Response)
		}
		mac.Write([]byte(authInfo.AuthorizedId))
		authInfo.Mac = mac.Sum(nil)
	}

	if err := ss.SendMsg(authInfo); err != nil {
		return nil, err
	}

	ctx := context.WithValue(ss.Context(), cluster.SharedKeysKey, sharedKeys)
	ctx = context.WithValue(ctx, cluster.ClusterIDKey, authInfo.AuthorizedId)

	return ctx, nil
}

func (a *Client) DoChallenge(clientStream streams.Stream) (context.Context, error) {
	cm, err := challenges.ClientMetadataFromOutgoingContext(clientStream.Context())
	if err != nil {
		return nil, err
	}
	var challengeRequests corev1.ChallengeRequestList
	if err := clientStream.RecvMsg(&challengeRequests); err != nil {
		return nil, err
	}
	if len(challengeRequests.Items) != 1 {
		return nil, status.Errorf(codes.Aborted, "invalid challenge request received from server")
	}
	req := challengeRequests.Items[0]

	challengeResponses := &corev1.ChallengeResponseList{
		Items: []*corev1.ChallengeResponse{
			challenges.Solve(req, cm, a.sharedKeys.ClientKey, DomainString),
		},
	}

	if err := clientStream.SendMsg(challengeResponses); err != nil {
		return nil, status.Errorf(codes.Aborted, "error sending challenge response: %v", err)
	}

	var authInfo corev1.AuthInfo
	if err := clientStream.RecvMsg(&authInfo); err != nil {
		if status.Code(err) == codes.Unknown {
			// EOF, etc
			return nil, status.Errorf(codes.Aborted, "error receiving auth info: %v", err)
		}
		return nil, err
	}

	{
		// verify the mac
		mac, _ := blake2b.New512(a.sharedKeys.ServerKey)
		mac.Write([]byte(cm.IdAssertion))
		mac.Write(cm.Random)
		for _, cr := range challengeRequests.Items {
			mac.Write(cr.Challenge)
		}
		for _, cr := range challengeResponses.Items {
			mac.Write(cr.Response)
		}
		mac.Write([]byte(authInfo.AuthorizedId))
		expectedMac := mac.Sum(nil)
		if subtle.ConstantTimeCompare(expectedMac, authInfo.Mac) != 1 {
			return nil, status.Errorf(codes.Aborted, "session info verification failed")
		}
	}

	ctx := context.WithValue(clientStream.Context(), cluster.SharedKeysKey, a.sharedKeys)
	ctx = context.WithValue(ctx, cluster.ClusterIDKey, authInfo.AuthorizedId)
	return ctx, nil
}

// Checks if the incoming context has the challenge version metadata key set to v2.
func ShouldEnableIncoming(streamContext context.Context) (bool, error) {
	if md, ok := metadata.FromIncomingContext(streamContext); ok {
		if v := md.Get(challenges.ChallengeVersionMetadataKey); len(v) > 0 {
			if v[0] == challenges.ChallengeV2 {
				return true, nil // v2
			}
			return false, fmt.Errorf("invalid challenge version %q", v[0])
		}
	}
	return false, nil // v1
}

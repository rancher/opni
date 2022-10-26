package cluster

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

type ClusterMiddleware struct {
	keyringStoreBroker storage.KeyringStoreBroker
	fakeKeyringStore   storage.KeyringStore
	headerKey          string
	logger             *zap.SugaredLogger
}

var _ auth.Middleware = (*ClusterMiddleware)(nil)

func New(ctx context.Context, keyringStore storage.KeyringStoreBroker, headerKey string) (*ClusterMiddleware, error) {
	lg := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("auth").Named("cluster")
	fakeKeyringStore, err := initFakeKeyring(ctx, keyringStore, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to set up keyring store: %w", err)
	}

	return &ClusterMiddleware{
		keyringStoreBroker: keyringStore,
		fakeKeyringStore:   fakeKeyringStore,
		headerKey:          headerKey,
		logger:             lg,
	}, nil
}

func initFakeKeyring(
	ctx context.Context,
	broker storage.KeyringStoreBroker,
	lg *zap.SugaredLogger,
) (storage.KeyringStore, error) {
	store := broker.KeyringStore("gateway-internal", &corev1.Reference{
		Id: "fake",
	})

	kp1 := ecdh.NewEphemeralKeyPair()
	kp2 := ecdh.NewEphemeralKeyPair()
	sec, err := ecdh.DeriveSharedSecret(kp1, ecdh.PeerPublicKey{
		PublicKey: kp2.PublicKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		return nil, err
	}
	fakeKeyring := keyring.New(keyring.NewSharedKeys(sec))
	go func() {
		p := backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(10*time.Millisecond),
			backoff.WithMaxInterval(10*time.Second),
			backoff.WithMultiplier(2.0),
		)
		bctx, ca := context.WithCancel(ctx)
		defer ca()
		b := p.Start(bctx)
		// print a warning every 10 failed attempts
		numFailedAttempts := 0
		for backoff.Continue(b) {
			ctx, ca := context.WithTimeout(bctx, 1*time.Second)
			defer ca()
			err := store.Put(ctx, fakeKeyring)
			if err == nil {
				if numFailedAttempts > 0 {
					lg.Infof("storage backend recovered after %d failed attempts", numFailedAttempts)
				}
				break
			}
			numFailedAttempts++
			if numFailedAttempts%10 == 0 {
				lg.With(
					"lastError", err,
					"attempt", numFailedAttempts,
				).Warn("the storage backend appears to be unresponsive, will continue to retry")
			}
		}
	}()
	return store, nil
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

func (m *ClusterMiddleware) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		nonce := uuid.New()
		if err := ss.SendMsg(&corev1.Challenge{
			Nonce: nonce[:],
		}); err != nil {
			return grpc.Errorf(codes.Aborted, "failed to send challenge: %v", err)
		}
		challengeResponse := &corev1.ChallengeResponse{}
		if err := ss.RecvMsg(challengeResponse); err != nil {
			return grpc.Errorf(codes.Aborted, "failed to receive challenge response: %v", err)
		}

		clusterID, sharedKeys, err := m.VerifyKeyring(challengeResponse.Authorization, nonce, []byte(info.FullMethod))
		if err != nil {
			return err
		}

		ctx := context.WithValue(ss.Context(), SharedKeysKey, sharedKeys)
		ctx = context.WithValue(ctx, ClusterIDKey, string(clusterID))

		return handler(srv, &util.ServerStreamWithContext{
			Stream: ss,
			Ctx:    ctx,
		})
	}
}

func (m *ClusterMiddleware) VerifyKeyring(authHeader string, expectedNonce uuid.UUID, msgBody []byte) (string, *keyring.SharedKeys, error) {
	lg := m.logger
	clusterID, nonce, mac, err := b2mac.DecodeAuthHeader(authHeader)
	if err != nil {
		return "", nil, util.StatusError(codes.InvalidArgument)
	}
	if subtle.ConstantTimeCompare(nonce[:], expectedNonce[:]) != 1 {
		lg.Debugf("unauthorized: mismatched nonce in auth header")
		return "", nil, util.StatusError(codes.Unauthenticated)
	}
	id := &corev1.Reference{
		Id: string(clusterID),
	}
	ks := m.keyringStoreBroker.KeyringStore("gateway", id)
	if kr, err := ks.Get(context.Background()); err == nil {
		authorized := false
		var sharedKeys *keyring.SharedKeys
		if ok := kr.Try(func(shared *keyring.SharedKeys) {
			if err := b2mac.Verify(mac, clusterID, nonce, msgBody, shared.ClientKey); err == nil {
				authorized = true
				sharedKeys = shared
			}
		}); !ok {
			lg.Errorf("unauthorized: invalid or corrupted keyring for cluster %s: %v", clusterID, err)
			return "", nil, util.StatusError(codes.Internal)
		}
		if !authorized {
			lg.Debugf("unauthorized: invalid mac for cluster %s", clusterID)
			return "", nil, util.StatusError(codes.Unauthenticated)
		}
		return string(clusterID), sharedKeys, nil
	}
	kr, err := m.fakeKeyringStore.Get(context.Background())
	if err != nil {
		lg.Errorf("failed to get fake keyring: %v", err)
		return "", nil, util.StatusError(codes.Internal)
	}
	kr.Try(func(shared *keyring.SharedKeys) {
		b2mac.Verify(mac, clusterID, nonce, msgBody, shared.ClientKey)
	})
	return "", nil, util.StatusError(codes.Unauthenticated)
}

func NewClientStreamInterceptor(id string, sharedKeys *keyring.SharedKeys) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return clientStream, err
		}
		challenge := &corev1.Challenge{}
		err = clientStream.RecvMsg(challenge)
		if err != nil {
			return nil, err
		}
		var nonce uuid.UUID
		if err := nonce.UnmarshalBinary(challenge.Nonce); err != nil {
			return nil, err
		}
		mac, err := b2mac.New512([]byte(id), nonce, []byte(method), sharedKeys.ClientKey)
		if err != nil {
			return nil, err
		}
		authHeader, err := b2mac.EncodeAuthHeader([]byte(id), nonce, mac)
		if err != nil {
			return nil, err
		}
		err = clientStream.SendMsg(&corev1.ChallengeResponse{
			Authorization: authHeader,
		})
		if err != nil {
			return nil, err
		}
		return clientStream, err
	}
}

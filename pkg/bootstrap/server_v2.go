package bootstrap

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	bootstrapv2 "github.com/rancher/opni/pkg/apis/bootstrap/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/health/annotations"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ServerV2 struct {
	bootstrapv2.UnsafeBootstrapServer
	privateKey     crypto.Signer
	storage        Storage
	clusterIdLocks util.LockMap[string, *sync.Mutex]
}

func NewServerV2(storage Storage, privateKey crypto.Signer) *ServerV2 {
	return &ServerV2{
		privateKey:     privateKey,
		storage:        storage,
		clusterIdLocks: util.NewLockMap[string, *sync.Mutex](),
	}
}

func (h *ServerV2) Join(ctx context.Context, _ *bootstrapv2.BootstrapJoinRequest) (*bootstrapv2.BootstrapJoinResponse, error) {
	signatures := map[string][]byte{}
	tokenList, err := h.storage.ListTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokenList {
		// Generate a JWS containing the signature of the detached secret token
		rawToken, err := tokens.FromBootstrapToken(token)
		if err != nil {
			return nil, err
		}
		sig, err := rawToken.SignDetached(h.privateKey)
		if err != nil {
			return nil, fmt.Errorf("error signing token: %w", err)
		}
		signatures[rawToken.HexID()] = sig
	}
	if len(signatures) == 0 {
		return nil, status.Error(codes.Unavailable, "server is not accepting bootstrap requests")
	}
	return &bootstrapv2.BootstrapJoinResponse{
		Signatures: signatures,
	}, nil
}

func (h *ServerV2) Auth(ctx context.Context, authReq *bootstrapv2.BootstrapAuthRequest) (*bootstrapv2.BootstrapAuthResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}
	var authHeader string
	if v := md.Get("Authorization"); len(v) > 0 {
		authHeader = strings.TrimSpace(v[0])
	} else {
		return nil, util.StatusError(codes.Unauthenticated)
	}
	if authHeader == "" {
		return nil, util.StatusError(codes.Unauthenticated)
	}
	// Authorization is given, check the authToken
	// Remove "Bearer " from the header
	bearerToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	// Verify the token
	payload, err := jws.Verify([]byte(bearerToken), jwa.EdDSA, h.privateKey.Public())
	if err != nil {
		return nil, util.StatusError(codes.PermissionDenied)
	}

	// The payload should contain the entire token encoded as JSON
	token, err := tokens.ParseJSON(payload)
	if err != nil {
		panic("bug: jws.Verify returned a malformed token")
	}
	bootstrapToken, err := h.storage.GetToken(ctx, token.Reference())
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, util.StatusError(codes.PermissionDenied)
		}
		return nil, util.StatusError(codes.Unavailable)
	}

	// after this point, we can return useful errors

	if err := validation.Validate(authReq); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// lock the mutex associated with the cluster ID
	// TODO: when scaling the gateway we need a distributed lock
	lock := h.clusterIdLocks.Get(authReq.ClientId)
	lock.Lock()
	defer lock.Unlock()

	existing := &corev1.Reference{
		Id: authReq.ClientId,
	}

	if cluster, err := h.storage.GetCluster(ctx, existing); err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "cluster %s already exists", cluster.Id)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.Unavailable, err.Error())
	}

	ekp := ecdh.NewEphemeralKeyPair()
	clientPubKey, err := ecdh.ClientPubKey(authReq)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, clientPubKey)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	kr := keyring.New(keyring.NewSharedKeys(sharedSecret))

	tokenLabels := maps.Clone(bootstrapToken.GetMetadata().GetLabels())
	delete(tokenLabels, corev1.NameLabel)
	if tokenLabels == nil {
		tokenLabels = map[string]string{}
	}
	tokenLabels[annotations.AgentVersion] = annotations.Version2
	if authReq.FriendlyName != nil {
		tokenLabels[corev1.NameLabel] = *authReq.FriendlyName
	}
	newCluster := &corev1.Cluster{
		Id: authReq.ClientId,
		Metadata: &corev1.ClusterMetadata{
			Labels: tokenLabels,
		},
	}
	if err := h.storage.CreateCluster(ctx, newCluster); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error creating cluster: %v", err))
	}

	_, err = h.storage.UpdateToken(ctx, token.Reference(),
		storage.NewCompositeMutator(
			storage.NewIncrementUsageCountMutator(),
		),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error incrementing usage count: %v", err))
	}

	krStore := h.storage.KeyringStore("gateway", newCluster.Reference())
	if err := krStore.Put(ctx, kr); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error storing keyring: %s", err))
	}

	return &bootstrapv2.BootstrapAuthResponse{
		ServerPubKey: ekp.PublicKey.Bytes(),
	}, nil
}

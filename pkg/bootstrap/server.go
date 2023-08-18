package bootstrap

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strings"
	"sync"

	"maps"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/health/annotations"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	bootstrapv1.UnsafeBootstrapServer
	privateKey      crypto.Signer
	storage         Storage
	capBackendStore capabilities.BackendStore
	clusterIdLocks  util.LockMap[string, *sync.Mutex]
}

func NewServer(storage Storage, privateKey crypto.Signer, capBackendStore capabilities.BackendStore) *Server {
	return &Server{
		privateKey:      privateKey,
		storage:         storage,
		capBackendStore: capBackendStore,
		clusterIdLocks:  util.NewLockMap[string, *sync.Mutex](),
	}
}

func (h *Server) Join(ctx context.Context, _ *bootstrapv1.BootstrapJoinRequest) (*bootstrapv1.BootstrapJoinResponse, error) {
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
	return &bootstrapv1.BootstrapJoinResponse{
		Signatures: signatures,
	}, nil
}

func (h *Server) Auth(ctx context.Context, authReq *bootstrapv1.BootstrapAuthRequest) (*bootstrapv1.BootstrapAuthResponse, error) {
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
	lock := h.clusterIdLocks.Get(authReq.ClientID)
	lock.Lock()
	defer lock.Unlock()

	// If the cluster with the requested ID does not exist, it can be created
	// normally. If it does exist, and the client advertises a capability that
	// the cluster does not yet have, and the token has the capability to edit
	// this cluster, the cluster will be updated with the new capability.
	existing := &corev1.Reference{
		Id: authReq.ClientID,
	}
	var shouldEditExisting bool

	if cluster, err := h.storage.GetCluster(ctx, existing); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
	} else {
		if capabilities.Has(cluster, capabilities.Cluster(authReq.Capability)) {
			return nil, status.Error(codes.AlreadyExists, "capability is already installed on this cluster")
		}

		// the cluster capability is new, check if the token can edit it
		if capabilities.Has(bootstrapToken, capabilities.JoinExistingCluster.For(existing)) {
			shouldEditExisting = true
		} else {
			return nil, status.Error(codes.PermissionDenied, "insufficient permissions for this cluster")
		}
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

	// Check if the capability exists and can be installed
	backendClient, err := h.capBackendStore.Get(authReq.Capability)
	if err != nil {
		if errors.Is(err, capabilities.ErrBackendNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if _, err := backendClient.CanInstall(ctx, &emptypb.Empty{}); err != nil {
		return nil, status.Errorf(codes.Unavailable, "capability %q cannot be installed: %v", authReq.Capability, err)
	}

	if shouldEditExisting {
		if err := h.handleEdit(ctx, existing, backendClient, bootstrapToken, kr); err != nil {
			return nil, status.Errorf(codes.Internal, "error installing capability %q: %v", authReq.Capability, err)
		}
	} else {
		tokenLabels := maps.Clone(bootstrapToken.GetMetadata().GetLabels())
		delete(tokenLabels, corev1.NameLabel)
		if tokenLabels == nil {
			tokenLabels = map[string]string{}
		}
		tokenLabels[annotations.AgentVersion] = annotations.Version1
		newCluster := &corev1.Cluster{
			Id: authReq.ClientID,
			Metadata: &corev1.ClusterMetadata{
				Labels:       tokenLabels,
				Capabilities: []*corev1.ClusterCapability{capabilities.Cluster(authReq.Capability)},
			},
		}
		if err := h.handleCreate(ctx, newCluster, backendClient, bootstrapToken, kr); err != nil {
			return nil, status.Errorf(codes.Internal, "error installing capability %q: %v", authReq.Capability, err)
		}
	}

	return &bootstrapv1.BootstrapAuthResponse{
		ServerPubKey: ekp.PublicKey.Bytes(),
	}, nil
}

func (h Server) handleCreate(
	ctx context.Context,
	newCluster *corev1.Cluster,
	newCapability capabilityv1.BackendClient,
	token *corev1.BootstrapToken,
	kr keyring.Keyring,
) error {
	if err := h.storage.CreateCluster(ctx, newCluster); err != nil {
		return fmt.Errorf("error creating cluster: %w", err)
	}
	_, err := h.storage.UpdateToken(ctx, token.Reference(),
		storage.NewCompositeMutator(
			storage.NewIncrementUsageCountMutator(),
			storage.NewAddCapabilityMutator[*corev1.BootstrapToken](&corev1.TokenCapability{
				Type:      string(capabilities.JoinExistingCluster),
				Reference: newCluster.Reference(),
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("error incrementing usage count: %w", err)
	}
	krStore := h.storage.KeyringStore("gateway", newCluster.Reference())
	if err := krStore.Put(ctx, kr); err != nil {
		return fmt.Errorf("error storing keyring: %w", err)
	}
	newCapability.Install(ctx, &capabilityv1.InstallRequest{
		Cluster: newCluster.Reference(),
	})
	return nil
}

func (h Server) handleEdit(
	ctx context.Context,
	existingCluster *corev1.Reference,
	newCapability capabilityv1.BackendClient,
	token *corev1.BootstrapToken,
	keyring keyring.Keyring,
) error {
	_, err := h.storage.UpdateToken(ctx, token.Reference(),
		storage.NewIncrementUsageCountMutator())
	if err != nil {
		return fmt.Errorf("error incrementing usage count: %w", err)
	}
	krStore := h.storage.KeyringStore("gateway", existingCluster)
	kr, err := krStore.Get(ctx)
	if err != nil {
		return fmt.Errorf("error getting existing keyring: %w", err)
	}
	if err := krStore.Put(ctx, keyring.Merge(kr)); err != nil {
		return fmt.Errorf("error storing keyring: %w", err)
	}
	_, err = newCapability.Install(ctx, &capabilityv1.InstallRequest{
		Cluster: existingCluster,
	})
	if err != nil {
		return err
	}
	return nil
}

package bootstrap

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Storage interface {
	storage.TokenStore
	storage.ClusterStore
	storage.KeyringStoreBroker
}

type StorageConfig struct {
	storage.TokenStore
	storage.ClusterStore
	storage.KeyringStoreBroker
}

type Server struct {
	bootstrapv1.UnsafeBootstrapServer
	privateKey crypto.Signer
	storage    Storage
	installer  capabilities.Installer
}

func NewServer(storage Storage, privateKey crypto.Signer, installer capabilities.Installer) *Server {
	return &Server{
		privateKey: privateKey,
		storage:    storage,
		installer:  installer,
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

func (h Server) Auth(ctx context.Context, authReq *bootstrapv1.BootstrapAuthRequest) (*bootstrapv1.BootstrapAuthResponse, error) {
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
	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: authReq.ClientPubKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	kr := keyring.New(keyring.NewSharedKeys(sharedSecret))

	// Check if the capability exists and can be installed
	if err := h.installer.CanInstall(authReq.Capability); err != nil {
		if errors.Is(err, capabilities.ErrUnknownCapability) {
			return nil, status.Errorf(codes.NotFound, "unknown capability %q", authReq.Capability)
		}
		return nil, status.Errorf(codes.Unavailable, "capability %q cannot be installed: %v", authReq.Capability, err)
	}

	if shouldEditExisting {
		if err := h.handleEdit(existing, authReq.Capability, bootstrapToken, kr); err != nil {
			return nil, status.Errorf(codes.Internal, "error installing capability %q: %v", authReq.Capability, err)
		}
	} else {
		newCluster := &corev1.Cluster{
			Id: authReq.ClientID,
			Metadata: &corev1.ClusterMetadata{
				Labels: bootstrapToken.GetMetadata().GetLabels(),
				Capabilities: []*corev1.ClusterCapability{
					{
						Name: authReq.Capability,
					},
				},
			},
		}
		if err := h.handleCreate(newCluster, authReq.Capability, bootstrapToken, kr); err != nil {
			return nil, status.Errorf(codes.Internal, "error installing capability %q: %v", authReq.Capability, err)
		}
	}

	return &bootstrapv1.BootstrapAuthResponse{
		ServerPubKey: ekp.PublicKey,
	}, nil
}

func (h Server) handleCreate(
	newCluster *corev1.Cluster,
	newCapability string,
	token *corev1.BootstrapToken,
	kr keyring.Keyring,
) error {
	if err := h.storage.CreateCluster(context.Background(), newCluster); err != nil {
		return fmt.Errorf("error creating cluster: %w", err)
	}
	_, err := h.storage.UpdateToken(context.Background(), token.Reference(),
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
	krStore, err := h.storage.KeyringStore("gateway", newCluster.Reference())
	if err != nil {
		return fmt.Errorf("error getting keyring store: %w", err)
	}
	if err := krStore.Put(context.Background(), kr); err != nil {
		return fmt.Errorf("error storing keyring: %w", err)
	}
	h.installer.InstallCapabilities(newCluster.Reference(), newCapability)
	return nil
}

func (h Server) handleEdit(
	existingCluster *corev1.Reference,
	newCapability string,
	token *corev1.BootstrapToken,
	keyring keyring.Keyring,
) error {
	_, err := h.storage.UpdateToken(context.Background(), token.Reference(),
		storage.NewIncrementUsageCountMutator())
	if err != nil {
		return fmt.Errorf("error incrementing usage count: %w", err)
	}
	_, err = h.storage.UpdateCluster(context.Background(), existingCluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(newCapability)),
	)
	if err != nil {
		return err
	}
	krStore, err := h.storage.KeyringStore("gateway", existingCluster)
	if err != nil {
		return fmt.Errorf("error getting keyring store: %w", err)
	}
	kr, err := krStore.Get(context.Background())
	if err != nil {
		return fmt.Errorf("error getting existing keyring: %w", err)
	}
	if err := krStore.Put(context.Background(), keyring.Merge(kr)); err != nil {
		return fmt.Errorf("error storing keyring: %w", err)
	}
	h.installer.InstallCapabilities(existingCluster, newCapability)
	return nil
}

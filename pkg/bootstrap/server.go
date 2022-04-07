package bootstrap

import (
	"context"
	"crypto"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	"github.com/rancher/opni-monitoring/pkg/capabilities"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	"github.com/rancher/opni-monitoring/pkg/validation"
)

type ServerConfig struct {
	Certificate         *tls.Certificate
	TokenStore          storage.TokenStore
	ClusterStore        storage.ClusterStore
	KeyringStoreBroker  storage.KeyringStoreBroker
	CapabilityInstaller capabilities.Installer
}

func (h ServerConfig) bootstrapJoinResponse(
	ctx context.Context,
) (BootstrapJoinResponse, error) {
	signatures := map[string][]byte{}
	tokenList, err := h.TokenStore.ListTokens(ctx)
	if err != nil {
		return BootstrapJoinResponse{}, err
	}
	for _, token := range tokenList {
		// Generate a JWS containing the signature of the detached secret token
		rawToken, err := tokens.FromBootstrapToken(token)
		if err != nil {
			return BootstrapJoinResponse{}, err
		}
		sig, err := rawToken.SignDetached(h.Certificate.PrivateKey)
		if err != nil {
			return BootstrapJoinResponse{}, fmt.Errorf("error signing token: %w", err)
		}
		signatures[rawToken.HexID()] = sig
	}
	return BootstrapJoinResponse{
		Signatures: signatures,
	}, nil
}

func (h ServerConfig) Handle(c *fiber.Ctx) error {
	switch c.Path() {
	case "/bootstrap/join":
		return h.handleBootstrapJoin(c)
	case "/bootstrap/auth":
		return h.handleBootstrapAuth(c)
	default:
		return c.SendStatus(fiber.StatusNotFound)
	}
}

func (h ServerConfig) handleBootstrapJoin(c *fiber.Ctx) error {
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if authHeader == "" {
		if resp, err := h.bootstrapJoinResponse(context.Background()); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		} else {
			if len(resp.Signatures) == 0 {
				// No tokens - server is not accepting bootstrap requests
				return c.SendStatus(fiber.StatusMethodNotAllowed)
			}
			return c.Status(fiber.StatusOK).JSON(resp)
		}
	} else {
		return c.SendStatus(fiber.StatusBadRequest)
	}
}

func (h ServerConfig) handleBootstrapAuth(c *fiber.Ctx) error {
	lg := c.Context().Logger()
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if strings.TrimSpace(authHeader) == "" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// Authorization is given, check the authToken
	// Remove "Bearer " from the header
	bearerToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	// Verify the token
	privKey := h.Certificate.PrivateKey.(crypto.Signer)
	payload, err := jws.Verify([]byte(bearerToken), jwa.EdDSA, privKey.Public())
	if err != nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// The payload should contain the entire token encoded as JSON
	token, err := tokens.ParseJSON(payload)
	if err != nil {
		panic("bug: jws.Verify returned a malformed token")
	}
	bootstrapToken, err := h.TokenStore.GetToken(context.Background(), token.Reference())
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		lg.Printf("error checking if token exists: %v")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Token is valid and not expired. Check the client's requested UUID
	clientReq := BootstrapAuthRequest{}
	if err := c.BodyParser(&clientReq); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request body")
	}
	if err := validation.Validate(clientReq); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	// If the cluster with the requested ID does not exist, it can be created
	// normally. If it does exist, and the client advertises a capability that
	// the cluster does not yet have, and the token has the capability to edit
	// this cluster, the cluster will be updated with the new capability.
	existing := &core.Reference{
		Id: clientReq.ClientID,
	}
	var shouldEditExisting bool

	if cluster, err := h.ClusterStore.GetCluster(context.Background(), existing); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			lg.Printf("error checking if cluster exists: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
	} else {
		if capabilities.Has(cluster, capabilities.Cluster(clientReq.Capability)) {
			return c.Status(fiber.StatusConflict).
				SendString("Capability is already installed on this cluster")
		}

		// the cluster capability is new, check if the token can edit it
		if capabilities.Has(bootstrapToken, capabilities.JoinExistingCluster.For(existing)) {
			shouldEditExisting = true
		} else {
			return c.Status(fiber.StatusUnauthorized).
				SendString("Insufficient permissions for this cluster")
		}
	}

	ekp := ecdh.NewEphemeralKeyPair()
	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: clientReq.ClientPubKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		lg.Printf("error computing shared secret: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	kr := keyring.New(keyring.NewSharedKeys(sharedSecret))

	// Check if the capability exists and can be installed
	if err := h.CapabilityInstaller.CanInstall(clientReq.Capability); err != nil {
		if errors.Is(err, capabilities.ErrUnknownCapability) {
			lg.Printf("unknown capability: %s", clientReq.Capability)
			return c.Status(fiber.StatusNotFound).
				SendString(fmt.Sprintf("Unknown capability %s", clientReq.Capability))
		}
		lg.Printf("capability cannot be installed: %v", err)
		return c.Status(fiber.StatusServiceUnavailable).
			SendString(fmt.Sprintf("Capability cannot be installed: %v", err))
	}

	if shouldEditExisting {
		if err := h.handleEdit(existing, clientReq.Capability, bootstrapToken, kr); err != nil {
			lg.Printf("error editing cluster capabilities: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
	} else {
		newCluster := &core.Cluster{
			Id: clientReq.ClientID,
			Metadata: &core.ClusterMetadata{
				Labels: bootstrapToken.GetMetadata().GetLabels(),
				Capabilities: []*core.ClusterCapability{
					{
						Name: clientReq.Capability,
					},
				},
			},
		}
		if err := h.handleCreate(newCluster, clientReq.Capability, bootstrapToken, kr); err != nil {
			lg.Printf("error creating cluster: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
	}

	return c.Status(fiber.StatusOK).JSON(BootstrapAuthResponse{
		ServerPubKey: ekp.PublicKey,
	})
}

func (h ServerConfig) handleCreate(
	newCluster *core.Cluster,
	newCapability string,
	token *core.BootstrapToken,
	kr keyring.Keyring,
) error {
	if err := h.ClusterStore.CreateCluster(context.Background(), newCluster); err != nil {
		return fmt.Errorf("error creating cluster: %w", err)
	}
	_, err := h.TokenStore.UpdateToken(context.Background(), token.Reference(),
		storage.NewCompositeMutator(
			storage.NewIncrementUsageCountMutator(),
			storage.NewAddCapabilityMutator[*core.BootstrapToken](&core.TokenCapability{
				Type:      string(capabilities.JoinExistingCluster),
				Reference: newCluster.Reference(),
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("error incrementing usage count: %w", err)
	}
	krStore, err := h.KeyringStoreBroker.KeyringStore(context.Background(), "gateway", newCluster.Reference())
	if err != nil {
		return fmt.Errorf("error getting keyring store: %w", err)
	}
	if err := krStore.Put(context.Background(), kr); err != nil {
		return fmt.Errorf("error storing keyring: %w", err)
	}
	h.CapabilityInstaller.InstallCapabilities(newCluster.Reference(), newCapability)
	return nil
}

func (h ServerConfig) handleEdit(
	existingCluster *core.Reference,
	newCapability string,
	token *core.BootstrapToken,
	keyring keyring.Keyring,
) error {
	_, err := h.TokenStore.UpdateToken(context.Background(), token.Reference(),
		storage.NewIncrementUsageCountMutator())
	if err != nil {
		return fmt.Errorf("error incrementing usage count: %w", err)
	}
	_, err = h.ClusterStore.UpdateCluster(context.Background(), existingCluster,
		storage.NewAddCapabilityMutator[*core.Cluster](capabilities.Cluster(newCapability)),
	)
	if err != nil {
		return err
	}
	krStore, err := h.KeyringStoreBroker.KeyringStore(context.Background(), "gateway", existingCluster)
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
	h.CapabilityInstaller.InstallCapabilities(existingCluster, newCapability)
	return nil
}

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
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

type ServerConfig struct {
	Certificate  *tls.Certificate
	TokenStore   storage.TokenStore
	ClusterStore storage.ClusterStore
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

	if _, err := h.ClusterStore.GetCluster(context.Background(), &core.Reference{
		Id: clientReq.ClientID,
	}); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			lg.Printf("error checking if cluster exists: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
	} else {
		return c.Status(fiber.StatusConflict).SendString("ID already in use")
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
	newCluster := &core.Cluster{
		Id:     clientReq.ClientID,
		Labels: bootstrapToken.GetMetadata().GetLabels(),
	}
	if err := h.ClusterStore.CreateCluster(context.Background(), newCluster); err != nil {
		lg.Printf("error creating cluster: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if err := h.TokenStore.IncrementUsageCount(context.Background(), token.Reference()); err != nil {
		lg.Printf("error incrementing usage count: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	krStore, err := h.ClusterStore.KeyringStore(context.Background(), newCluster.Reference())
	if err != nil {
		lg.Printf("error getting keyring store: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if err := krStore.Put(context.Background(), kr); err != nil {
		lg.Printf("error storing keyring: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.Status(fiber.StatusOK).JSON(BootstrapAuthResponse{
		ServerPubKey: ekp.PublicKey,
	})
}

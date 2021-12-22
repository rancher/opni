package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-gateway/pkg/ecdh"
	"github.com/kralicky/opni-gateway/pkg/keyring"
	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
)

type ServerConfig struct {
	RootCA     *x509.Certificate
	Keypair    *tls.Certificate
	TokenStore storage.TokenStore
}

func (h ServerConfig) bootstrapResponse(
	ctx context.Context,
) (BootstrapResponse, error) {
	var caCert []byte
	if h.RootCA != nil {
		caCert = h.RootCA.Raw
	}
	signatures := map[string][]byte{}
	tokenIDs, err := h.TokenStore.ListTokens(ctx)
	if err != nil {
		return BootstrapResponse{}, err
	}
	for _, id := range tokenIDs {
		token, err := h.TokenStore.GetToken(ctx, id)
		if err != nil {
			log.Println(err)
			continue
		}
		// Generate a JWS containing the signature of the detached secret token
		sig, err := token.SignDetached(h.Keypair.PrivateKey)
		if err != nil {
			return BootstrapResponse{}, fmt.Errorf("error signing token: %w", err)
		}
		signatures[token.HexID()] = sig
	}
	return BootstrapResponse{
		CACert:     caCert,
		Signatures: signatures,
	}, nil
}

func (h ServerConfig) Handle(c *fiber.Ctx) error {
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if authHeader == "" {
		if resp, err := h.bootstrapResponse(c.Context()); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		} else {
			return c.Status(fiber.StatusOK).JSON(resp)
		}
	}
	// Authorization is given, check the authToken
	// Remove "Bearer " from the header
	bearerToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	// Verify the token
	edPrivKey := h.Keypair.PrivateKey.(ed25519.PrivateKey)
	_, err := jws.Verify([]byte(bearerToken), jwa.EdDSA, edPrivKey.Public())
	if err != nil {
		log.Printf("error verifying token: %v", err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Token is valid. Check the client's requested UUID
	clientReq := SecureBootstrapRequest{}
	if err := c.BodyParser(&clientReq); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	ekp, err := ecdh.NewEphemeralKeyPair()
	if err != nil {
		log.Printf("error generating server keypair: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: clientReq.ClientPubKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		log.Printf("error computing shared secret: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	_ = keyring.New(keyring.NewClientKeys(sharedSecret))

	return c.Status(fiber.StatusOK).JSON(SecureBootstrapResponse{
		ServerPubKey: ekp.PublicKey,
	})
}

package bootstrap

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"k8s.io/client-go/rest"
)

var (
	ErrInvalidEndpoint    = errors.New("invalid endpoint")
	ErrNoRootCA           = errors.New("no root CA found in peer certificates")
	ErrLeafNotSigned      = errors.New("leaf certificate not signed by the root CA")
	ErrKeyExpired         = errors.New("key expired")
	ErrRootCAHashMismatch = errors.New("root CA hash mismatch")
	ErrNoValidSignature   = errors.New("no valid signature found in response")
	ErrNoToken            = errors.New("no bootstrap token provided")
)

type ClientConfig struct {
	Capability    string
	Token         *tokens.Token
	Endpoint      string
	DialOpts      []grpc.DialOption
	K8sConfig     *rest.Config
	K8sNamespace  string
	TrustStrategy trust.Strategy
}

func (c *ClientConfig) Bootstrap(
	ctx context.Context,
	ident ident.Provider,
) (keyring.Keyring, error) {
	if c.Token == nil {
		return nil, ErrNoToken
	}
	response, serverLeafCert, err := c.bootstrapJoin(ctx)
	if err != nil {
		return nil, err
	}

	completeJws, err := c.findValidSignature(
		response.Signatures, serverLeafCert.PublicKey)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := c.TrustStrategy.TLSConfig()
	if err != nil {
		return nil, err
	}

	cc, err := grpc.DialContext(ctx, c.Endpoint,
		append(c.DialOpts,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		)...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial gateway: %w", err)
	}
	client := bootstrapv1.NewBootstrapClient(cc)

	ekp := ecdh.NewEphemeralKeyPair()
	id, err := ident.UniqueIdentifier(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain unique identifier: %w", err)
	}
	authReq := &bootstrapv1.BootstrapAuthRequest{
		ClientID:     id,
		ClientPubKey: ekp.PublicKey,
		Capability:   c.Capability,
	}

	authResp, err := client.Auth(metadata.NewOutgoingContext(ctx, metadata.Pairs(
		auth.AuthorizationKey, "Bearer "+string(completeJws),
	)), authReq)
	if err != nil {
		return nil, fmt.Errorf("auth request failed: %w", err)
	}

	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: authResp.ServerPubKey,
		PeerType:  ecdh.PeerTypeServer,
	})
	if err != nil {
		return nil, err
	}

	keys := []any{keyring.NewSharedKeys(sharedSecret)}
	if k := c.TrustStrategy.PersistentKey(); k != nil {
		keys = append(keys, k)
	}
	return keyring.New(keys...), nil
}

func (c *ClientConfig) Finalize(ctx context.Context) error {
	ns := c.K8sNamespace
	if ns == "" {
		if nsEnv, ok := os.LookupEnv("POD_NAMESPACE"); ok {
			ns = nsEnv
		} else {
			return errors.New("POD_NAMESPACE not set, and no namespace was explicitly configured")
		}
	}
	return eraseBootstrapTokensFromConfig(ctx, c.K8sConfig, ns)
}

func (c *ClientConfig) bootstrapJoin(ctx context.Context) (*bootstrapv1.BootstrapJoinResponse, *x509.Certificate, error) {
	tlsConfig, err := c.TrustStrategy.TLSConfig()
	if err != nil {
		return nil, nil, err
	}
	cc, err := grpc.DialContext(ctx, c.Endpoint,
		append(c.DialOpts,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		)...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial gateway: %w", err)
	}
	client := bootstrapv1.NewBootstrapClient(cc)

	var peer peer.Peer
	resp, err := client.Join(ctx, &bootstrapv1.BootstrapJoinRequest{}, grpc.Peer(&peer))
	if err != nil {
		return nil, nil, fmt.Errorf("join request failed: %w", err)
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected type for peer TLS info: %#T", peer.AuthInfo)
	}

	return resp, tlsInfo.State.PeerCertificates[0], nil
}

func (c *ClientConfig) findValidSignature(
	signatures map[string][]byte,
	pubKey interface{},
) ([]byte, error) {
	if sig, ok := signatures[c.Token.HexID()]; ok {
		return c.Token.VerifyDetached(sig, pubKey)
	}
	return nil, ErrNoValidSignature
}

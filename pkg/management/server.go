package management

import (
	context "context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/kralicky/opni-gateway/pkg/util"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	DefaultManagementSocket = "/tmp/opni-gateway.sock"
	DefaultTokenTTL         = 2 * time.Minute
)

type Server struct {
	UnimplementedManagementServer
	ManagementServerOptions
}

type ManagementServerOptions struct {
	socket     string
	tokenStore storage.TokenStore
	tlsConfig  *tls.Config
}

type ManagementServerOption func(*ManagementServerOptions)

func (o *ManagementServerOptions) Apply(opts ...ManagementServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func Socket(socket string) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.socket = socket
	}
}

func TokenStore(tokenStore storage.TokenStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.tokenStore = tokenStore
	}
}

func TLSConfig(config *tls.Config) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.tlsConfig = config
	}
}

func NewServer(opts ...ManagementServerOption) *Server {
	options := ManagementServerOptions{
		socket: DefaultManagementSocket,
	}
	options.Apply(opts...)
	if options.tokenStore == nil {
		panic("token store is required")
	}
	return &Server{
		ManagementServerOptions: options,
	}
}

func (m *Server) ListenAndServe(ctx context.Context) error {
	if err := m.createSocketDir(); err != nil {
		return err
	}
	if _, err := os.Stat(m.socket); err == nil {
		if err := os.Remove(m.socket); err != nil {
			return err
		}
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "unix", m.socket)
	if err != nil {
		return err
	}
	fmt.Println("Listening on", m.socket)
	srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	RegisterManagementServer(srv, m)
	return srv.Serve(listener)
}

func (m *Server) createSocketDir() error {
	if _, err := os.Stat(filepath.Dir(m.socket)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(m.socket), 0700); err != nil {
			return err
		}
	}
	return nil
}

func (m *Server) CreateBootstrapToken(
	ctx context.Context,
	req *CreateBootstrapTokenRequest,
) (*BootstrapToken, error) {
	ttl := DefaultTokenTTL
	if req.TTL != nil {
		ttl = req.GetTTL().AsDuration()
	}
	token, err := m.tokenStore.CreateToken(ctx, ttl)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return BootstrapTokenFromToken(token), nil
}

func (m *Server) RevokeBootstrapToken(
	ctx context.Context,
	req *RevokeBootstrapTokenRequest,
) (*emptypb.Empty, error) {
	if err := m.tokenStore.DeleteToken(ctx, req.GetTokenID()); err != nil {
		if errors.Is(err, storage.ErrTokenNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (m *Server) ListBootstrapTokens(
	ctx context.Context,
	req *ListBootstrapTokensRequest,
) (*ListBootstrapTokensResponse, error) {
	tokens, err := m.tokenStore.ListTokens(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tokenList := make([]*BootstrapToken, len(tokens))
	for i, token := range tokens {
		tokenList[i] = BootstrapTokenFromToken(token)
	}
	return &ListBootstrapTokensResponse{
		Tokens: tokenList,
	}, nil
}

func (m *Server) ListTenants(
	ctx context.Context,
	req *emptypb.Empty,
) (*ListTenantsResponse, error) {
	return &ListTenantsResponse{
		Tenants: []*Tenant{},
	}, nil
}

func (m *Server) CertsInfo(
	ctx context.Context,
	req *emptypb.Empty,
) (*CertsInfoResponse, error) {
	resp := &CertsInfoResponse{
		Chain: []*CertInfo{},
	}
	for _, tlsCert := range m.tlsConfig.Certificates[:1] {
		for _, der := range tlsCert.Certificate {
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			resp.Chain = append(resp.Chain, &CertInfo{
				Issuer:    cert.Issuer.String(),
				Subject:   cert.Subject.String(),
				IsCA:      cert.IsCA,
				NotBefore: cert.NotBefore.Format(time.RFC3339),
				NotAfter:  cert.NotAfter.Format(time.RFC3339),
				SPKIHash:  util.CertSPKIHash(cert),
			})
		}
	}
	return resp, nil
}

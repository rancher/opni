package management

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/kralicky/opni-gateway/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	DefaultTokenTTL = 2 * time.Minute
)

type Server struct {
	UnimplementedManagementServer
	ManagementServerOptions
}

type ManagementServerOptions struct {
	listenAddress string
	tokenStore    storage.TokenStore
	tenantStore   storage.TenantStore
	tlsConfig     *tls.Config
}

type ManagementServerOption func(*ManagementServerOptions)

func (o *ManagementServerOptions) Apply(opts ...ManagementServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func ListenAddress(socket string) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		if socket != "" {
			o.listenAddress = socket
		}
	}
}

func TokenStore(tokenStore storage.TokenStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.tokenStore = tokenStore
	}
}

func TenantStore(tenantStore storage.TenantStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.tenantStore = tenantStore
	}
}

func TLSConfig(config *tls.Config) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.tlsConfig = config
	}
}

func NewServer(opts ...ManagementServerOption) *Server {
	options := ManagementServerOptions{
		listenAddress: DefaultManagementSocket(),
	}
	options.Apply(opts...)
	if options.tokenStore == nil || options.tenantStore == nil {
		panic("token store and tenant store are required")
	}
	return &Server{
		ManagementServerOptions: options,
	}
}

func (m *Server) ListenAndServe(ctx context.Context) error {
	listener, err := util.NewProtocolListener(ctx, m.listenAddress)
	if err != nil {
		return err
	}
	fmt.Printf("Management API listening on %s\n", m.listenAddress)
	srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	RegisterManagementServer(srv, m)
	return srv.Serve(listener)
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
		if errors.Is(err, storage.ErrNotFound) {
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
	tenants, err := m.tenantStore.ListTenants(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tenantList := make([]*Tenant, len(tenants))
	for i, tenantID := range tenants {
		tenantList[i] = &Tenant{
			ID: tenantID,
		}
	}
	return &ListTenantsResponse{
		Tenants: tenantList,
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

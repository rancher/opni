package management

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/util"
	"go.uber.org/zap"
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
	logger *zap.SugaredLogger
}

type ManagementServerOptions struct {
	listenAddress string
	tokenStore    storage.TokenStore
	tenantStore   storage.TenantStore
	rbacStore     storage.RBACStore
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

func RBACStore(rbacStore storage.RBACStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.rbacStore = rbacStore
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
		logger:                  logger.New().Named("mgmt"),
	}
}

func (m *Server) ListenAndServe(ctx context.Context) error {
	listener, err := util.NewProtocolListener(ctx, m.listenAddress)
	if err != nil {
		return err
	}
	m.logger.With(
		"address", listener.Addr().String(),
	).Info("management server starting")
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
	return NewBootstrapToken(token), nil
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
		tokenList[i] = NewBootstrapToken(token)
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

func (m *Server) DeleteTenant(
	ctx context.Context,
	tenant *Tenant,
) (*emptypb.Empty, error) {
	err := m.tenantStore.DeleteTenant(ctx, tenant.ID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &emptypb.Empty{}, nil
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
				Issuer:      cert.Issuer.String(),
				Subject:     cert.Subject.String(),
				IsCA:        cert.IsCA,
				NotBefore:   cert.NotBefore.Format(time.RFC3339),
				NotAfter:    cert.NotAfter.Format(time.RFC3339),
				Fingerprint: pkp.NewSha256(cert).Encode(),
			})
		}
	}
	return resp, nil
}

func (m *Server) CreateRole(
	ctx context.Context,
	in *CreateRoleRequest,
) (*Role, error) {
	if role, err := m.rbacStore.CreateRole(ctx, in.Role.Name, in.Role.TenantIDs); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return NewRole(role), nil
	}
}

func (m *Server) DeleteRole(
	ctx context.Context,
	in *DeleteRoleRequest,
) (*emptypb.Empty, error) {
	if err := m.rbacStore.DeleteRole(ctx, in.Name); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (m *Server) GetRole(
	ctx context.Context,
	in *GetRoleRequest,
) (*Role, error) {
	if role, err := m.rbacStore.GetRole(ctx, in.Name); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return NewRole(role), nil
	}
}

func (m *Server) CreateRoleBinding(
	ctx context.Context,
	in *CreateRoleBindingRequest,
) (*RoleBinding, error) {
	if roleBinding, err := m.rbacStore.CreateRoleBinding(ctx,
		in.RoleBinding.Name,
		in.RoleBinding.RoleName,
		in.RoleBinding.UserID,
	); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return NewRoleBinding(roleBinding), nil
	}
}

func (m *Server) DeleteRoleBinding(
	ctx context.Context,
	in *DeleteRoleBindingRequest,
) (*emptypb.Empty, error) {
	if err := m.rbacStore.DeleteRoleBinding(ctx, in.Name); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (m *Server) GetRoleBinding(
	ctx context.Context,
	in *GetRoleBindingRequest,
) (*RoleBinding, error) {
	if roleBinding, err := m.rbacStore.GetRoleBinding(ctx, in.Name); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return NewRoleBinding(roleBinding), nil
	}
}

func (m *Server) ListRoles(
	ctx context.Context,
	in *emptypb.Empty,
) (*RoleList, error) {
	if roles, err := m.rbacStore.ListRoles(ctx); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		list := &RoleList{
			Items: make([]*Role, len(roles)),
		}
		for i, role := range roles {
			list.Items[i] = NewRole(role)
		}
		return list, nil
	}
}

func (m *Server) ListRoleBindings(
	ctx context.Context,
	in *emptypb.Empty,
) (*RoleBindingList, error) {
	if roleBindings, err := m.rbacStore.ListRoleBindings(ctx); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		list := &RoleBindingList{
			Items: make([]*RoleBinding, len(roleBindings)),
		}
		for i, roleBinding := range roleBindings {
			list.Items[i] = NewRoleBinding(roleBinding)
		}
		return list, nil
	}
}

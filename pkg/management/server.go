package management

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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
	clusterStore  storage.ClusterStore
	storage.RBACStore
	tlsConfig *tls.Config
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

func TenantStore(tenantStore storage.ClusterStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.clusterStore = tenantStore
	}
}

func RBACStore(rbacStore storage.RBACStore) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.RBACStore = rbacStore
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
	if options.tokenStore == nil || options.clusterStore == nil {
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
	go func() {
		<-ctx.Done()
		srv.Stop()
	}()
	return srv.Serve(listener)
}

func (m *Server) CertsInfo(ctx context.Context) (*CertsInfoResponse, error) {
	resp := &CertsInfoResponse{
		Chain: []*core.CertInfo{},
	}
	for _, tlsCert := range m.tlsConfig.Certificates[:1] {
		for _, der := range tlsCert.Certificate {
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			resp.Chain = append(resp.Chain, &core.CertInfo{
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

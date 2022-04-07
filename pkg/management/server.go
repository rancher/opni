package management

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/kralicky/grpc-gateway/v2/runtime"
	"github.com/rancher/opni-monitoring/pkg/capabilities"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/pkp"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

//go:embed management.swagger.json
var managementSwaggerJson []byte

func OpenAPISpec() []byte {
	buf := make([]byte, len(managementSwaggerJson))
	copy(buf, managementSwaggerJson)
	return buf
}

// CoreDataSource provides a way to obtain data which the management
// server needs to serve its core API
type CoreDataSource interface {
	StorageBackend() storage.Backend
	TLSConfig() *tls.Config
}

// CapabilitiesDataSource provides a way to obtain data which the management
// server needs to serve capabilities-related endpoints
type CapabilitiesDataSource interface {
	CapabilitiesStore() capabilities.BackendStore
}

type apiExtension struct {
	client      apiextensions.ManagementAPIExtensionClient
	clientConn  *grpc.ClientConn
	serviceDesc *desc.ServiceDescriptor
	httpRules   []*HTTPRuleDescriptor
}

type Server struct {
	UnsafeManagementServer
	ManagementServerOptions
	config         *v1beta1.ManagementSpec
	logger         *zap.SugaredLogger
	rbacProvider   rbac.Provider
	ctx            context.Context
	coreDataSource CoreDataSource

	apiExtensions []apiExtension
}

var _ ManagementServer = (*Server)(nil)

type APIExtensionPlugin = plugins.TypedActivePlugin[apiextensions.ManagementAPIExtensionClient]

type ManagementServerOptions struct {
	lifecycler             config.Lifecycler
	apiExtPlugins          []APIExtensionPlugin
	systemPlugins          []plugins.ActivePlugin
	capabilitiesDataSource CapabilitiesDataSource
}

type ManagementServerOption func(*ManagementServerOptions)

func (o *ManagementServerOptions) Apply(opts ...ManagementServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithAPIExtensions(exts []APIExtensionPlugin) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.apiExtPlugins = append(o.apiExtPlugins, exts...)
	}
}

func WithSystemPlugins(plugins []plugins.ActivePlugin) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.systemPlugins = plugins
	}
}

func WithLifecycler(lc config.Lifecycler) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.lifecycler = lc
	}
}

func WithCapabilitiesDataSource(src CapabilitiesDataSource) ManagementServerOption {
	return func(o *ManagementServerOptions) {
		o.capabilitiesDataSource = src
	}
}

func NewServer(
	ctx context.Context,
	conf *v1beta1.ManagementSpec,
	cds CoreDataSource,
	opts ...ManagementServerOption,
) *Server {
	lg := logger.New().Named("mgmt")
	options := ManagementServerOptions{
		lifecycler: config.NewUnavailableLifecycler(meta.ObjectList{}),
	}
	options.Apply(opts...)

	return &Server{
		ManagementServerOptions: options,
		ctx:                     ctx,
		config:                  conf,
		logger:                  lg,
		coreDataSource:          cds,
		rbacProvider:            storage.NewRBACProvider(cds.StorageBackend()),
	}
}

type managementApiServer interface {
	ServeManagementAPI(ManagementServer)
}

func (m *Server) ListenAndServe() error {
	if m.config.GRPCListenAddress == "" {
		return errors.New("GRPCListenAddress not configured")
	}
	lg := m.logger
	listener, err := util.NewProtocolListener(m.config.GRPCListenAddress)
	if err != nil {
		return err
	}
	lg.With(
		"address", listener.Addr().String(),
	).Info("management gRPC server starting")
	director := m.configureApiExtensionDirector(m.ctx)
	srv := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
		grpc.UnknownServiceHandler(unknownServiceHandler(director)),
	)
	RegisterManagementServer(srv, m)

	for _, plugin := range m.systemPlugins {
		go plugin.Raw.(managementApiServer).ServeManagementAPI(m)
	}
	waitctx.Go(m.ctx, func() {
		<-m.ctx.Done()
		srv.GracefulStop()
	})
	if m.config.HTTPListenAddress != "" {
		go m.listenAndServeHttp(listener)
	}

	waitctx.AddOne(m.ctx)
	defer waitctx.Done(m.ctx)
	return srv.Serve(listener)
}

func (m *Server) listenAndServeHttp(listener net.Listener) {
	lg := m.logger
	lg.With(
		"address", m.config.HTTPListenAddress,
	).Info("management HTTP server starting")
	mux := http.NewServeMux()
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(OpenAPISpec()); err != nil {
			lg.Error(err)
		}
	})
	gwmux := runtime.NewServeMux()
	if err := RegisterManagementHandlerFromEndpoint(m.ctx, gwmux, listener.Addr().String(),
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}); err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to register management handler")
	}
	m.configureHttpApiExtensions(gwmux)
	mux.Handle("/", gwmux)
	server := &http.Server{
		Addr:    m.config.HTTPListenAddress,
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return m.ctx
		},
	}
	waitctx.Go(m.ctx, func() {
		<-m.ctx.Done()
		if err := server.Close(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to close http gateway")
		}
	})
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		lg.With(
			zap.Error(err),
		).Error("http gateway exited with error")
	}
}

func (m *Server) CertsInfo(ctx context.Context, _ *emptypb.Empty) (*CertsInfoResponse, error) {
	resp := &CertsInfoResponse{
		Chain: []*core.CertInfo{},
	}
	for _, tlsCert := range m.coreDataSource.TLSConfig().Certificates[:1] {
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

func (m *Server) ListCapabilities(ctx context.Context, _ *emptypb.Empty) (*CapabilityList, error) {
	if m.capabilitiesDataSource == nil {
		return nil, status.Error(codes.Unavailable, "capability backend store not configured")
	}

	return &CapabilityList{
		Items: m.capabilitiesDataSource.CapabilitiesStore().List(),
	}, nil
}

func (m *Server) CapabilityInstaller(
	ctx context.Context,
	req *CapabilityInstallerRequest,
) (*CapabilityInstallerResponse, error) {
	if m.capabilitiesDataSource == nil {
		return nil, status.Error(codes.Unavailable, "capability backend store not configured")
	}

	cmd, err := m.capabilitiesDataSource.
		CapabilitiesStore().
		RenderInstaller(req.Name, capabilities.UserInstallerTemplateSpec{
			Token: req.Token,
			Pin:   req.Pin,
		})
	if err != nil {
		return nil, err
	}
	return &CapabilityInstallerResponse{
		Command: cmd,
	}, nil
}

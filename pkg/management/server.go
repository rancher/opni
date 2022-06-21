package management

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/kralicky/grpc-gateway/v2/runtime"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

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

type HealthStatusDataSource interface {
	ClusterHealthStatus(ref *corev1.Reference) (*corev1.HealthStatus, error)
}

type apiExtension struct {
	client      apiextensions.ManagementAPIExtensionClient
	clientConn  *grpc.ClientConn
	serviceDesc *desc.ServiceDescriptor
	httpRules   []*managementv1.HTTPRuleDescriptor
}

type Server struct {
	managementv1.UnsafeManagementServer
	managementServerOptions
	config         *v1beta1.ManagementSpec
	logger         *zap.SugaredLogger
	rbacProvider   rbac.Provider
	coreDataSource CoreDataSource
	grpcServer     *grpc.Server

	apiExtMu      sync.RWMutex
	apiExtensions []apiExtension
}

var _ managementv1.ManagementServer = (*Server)(nil)

type managementServerOptions struct {
	lifecycler             config.Lifecycler
	capabilitiesDataSource CapabilitiesDataSource
	healthStatusDataSource HealthStatusDataSource
}

type ManagementServerOption func(*managementServerOptions)

func (o *managementServerOptions) apply(opts ...ManagementServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLifecycler(lc config.Lifecycler) ManagementServerOption {
	return func(o *managementServerOptions) {
		o.lifecycler = lc
	}
}

func WithCapabilitiesDataSource(src CapabilitiesDataSource) ManagementServerOption {
	return func(o *managementServerOptions) {
		o.capabilitiesDataSource = src
	}
}

func WithHealthStatusDataSource(src HealthStatusDataSource) ManagementServerOption {
	return func(o *managementServerOptions) {
		o.healthStatusDataSource = src
	}
}

func NewServer(
	ctx waitctx.RestrictiveContext,
	conf *v1beta1.ManagementSpec,
	cds CoreDataSource,
	pluginLoader plugins.LoaderInterface,
	opts ...ManagementServerOption,
) *Server {
	lg := logger.New().Named("mgmt")
	options := managementServerOptions{
		lifecycler: config.NewUnavailableLifecycler(cfgmeta.ObjectList{}),
	}
	options.apply(opts...)

	m := &Server{
		managementServerOptions: options,
		config:                  conf,
		logger:                  lg,
		coreDataSource:          cds,
		rbacProvider:            storage.NewRBACProvider(cds.StorageBackend()),
	}

	director := m.configureApiExtensionDirector(ctx, pluginLoader)
	m.grpcServer = grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
		grpc.UnknownServiceHandler(unknownServiceHandler(director)),
		grpc.ChainStreamInterceptor(otelgrpc.StreamServerInterceptor()),
		grpc.ChainUnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
	)
	managementv1.RegisterManagementServer(m.grpcServer, m)

	pluginLoader.Hook(hooks.OnLoad(func(sp types.SystemPlugin) {
		go sp.ServeManagementAPI(m)
		go sp.ServeAPIExtensions(m.config.GRPCListenAddress)
	}))

	return m
}

type managementApiServer interface {
	ServeManagementAPI(managementv1.ManagementServer)
}

func (m *Server) ListenAndServe(ctx waitctx.RestrictiveContext) error {
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

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		m.grpcServer.GracefulStop()
	})
	if m.config.HTTPListenAddress != "" {
		go m.listenAndServeHttp(ctx, listener)
	}

	waitctx.AddOne(ctx)
	defer waitctx.Done(ctx)
	return m.grpcServer.Serve(listener)
}

func (m *Server) listenAndServeHttp(ctx waitctx.RestrictiveContext, listener net.Listener) {
	lg := m.logger
	lg.With(
		"address", m.config.HTTPListenAddress,
	).Info("management HTTP server starting")
	mux := http.NewServeMux()
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(managementv1.OpenAPISpec()); err != nil {
			lg.Error(err)
		}
	})
	gwmux := runtime.NewServeMux()
	if err := managementv1.RegisterManagementHandlerFromEndpoint(ctx, gwmux, listener.Addr().String(),
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
			return ctx
		},
	}
	waitctx.Go(ctx, func() {
		<-ctx.Done()
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

func (m *Server) CertsInfo(ctx context.Context, _ *emptypb.Empty) (*managementv1.CertsInfoResponse, error) {
	resp := &managementv1.CertsInfoResponse{
		Chain: []*corev1.CertInfo{},
	}
	for _, tlsCert := range m.coreDataSource.TLSConfig().Certificates[:1] {
		for _, der := range tlsCert.Certificate {
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			resp.Chain = append(resp.Chain, &corev1.CertInfo{
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

func (m *Server) ListCapabilities(ctx context.Context, _ *emptypb.Empty) (*managementv1.CapabilityList, error) {
	if m.capabilitiesDataSource == nil {
		return nil, status.Error(codes.Unavailable, "capability backend store not configured")
	}

	return &managementv1.CapabilityList{
		Items: m.capabilitiesDataSource.CapabilitiesStore().List(),
	}, nil
}

func (m *Server) CapabilityInstaller(
	ctx context.Context,
	req *managementv1.CapabilityInstallerRequest,
) (*managementv1.CapabilityInstallerResponse, error) {
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
	return &managementv1.CapabilityInstallerResponse{
		Command: cmd,
	}, nil
}

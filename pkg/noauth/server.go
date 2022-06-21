package noauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/lestrrat-go/jwx/jwk"
	"github.com/ory/fosite"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServerConfig struct {
	Issuer                string              `json:"issuer,omitempty"`
	ClientID              string              `json:"clientID,omitempty"`
	ClientSecret          string              `json:"clientSecret,omitempty"`
	GrafanaHostname       string              `json:"grafanaHostname,omitempty"`
	RedirectURI           string              `json:"redirectURI,omitempty"`
	ManagementAPIEndpoint string              `json:"managementAPIEndpoint,omitempty"`
	Port                  int                 `json:"port,omitempty"`
	Debug                 bool                `json:"debug,omitempty"`
	OpenID                openid.OpenidConfig `json:"openid,omitempty"`

	Logger *zap.SugaredLogger `json:"-"`
}

type Server struct {
	ServerConfig
	mgmtApiClient  managementv1.ManagementClient
	noauthProvider fosite.OAuth2Provider
	key            jwk.Key
}

func NewServer(conf *ServerConfig) *Server {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	key, err := jwk.New(privKey)
	if err != nil {
		panic(err)
	}
	if err := jwk.AssignKeyID(key); err != nil {
		panic(err)
	}

	provider := newOAuthProvider(conf, privKey)
	return &Server{
		ServerConfig:   *conf,
		noauthProvider: provider,
		key:            key,
	}
}

type templateData struct {
	Users []string
}

//go:embed web
var webFS embed.FS

func (s *Server) Run(ctx waitctx.RestrictiveContext) error {
	lg := s.Logger
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", s.Port))
	if err != nil {
		return err
	}

	lg.With(
		"address", listener.Addr(),
	).Info("noauth server starting")

	mux := http.NewServeMux()

	if err := s.connectToManagementAPI(ctx); err != nil {
		return err
	}

	s.configureOAuthServer(mux)
	s.configureWebServer(ctx, mux)

	server := http.Server{
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("noauth server shutting down")
		if err := server.Close(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("an error occurred while shutting down the server")
		}
	})
	err = server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) connectToManagementAPI(ctx context.Context) error {
	lg := s.Logger
	lg.With(
		"address", s.ManagementAPIEndpoint,
	).Info("connecting to management api")
	cc, err := grpc.DialContext(ctx, s.ManagementAPIEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	lg.Info("connected to management api")
	waitctx.Go(ctx, func() {
		<-ctx.Done()
		cc.Close()
	})
	s.mgmtApiClient = managementv1.NewManagementClient(cc)
	return nil
}

func (s *Server) configureWebServer(ctx context.Context, mux *http.ServeMux) {
	mux.Handle("/web/", http.FileServer(http.FS(webFS)))
}

func (in *ServerConfig) DeepCopyInto(out *ServerConfig) {
	util.DeepCopyInto(out, in)
}

func (in *ServerConfig) DeepCopy() *ServerConfig {
	return util.DeepCopy(in)
}

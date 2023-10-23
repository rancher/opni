package noauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"embed"
	"fmt"
	"net"
	"net/http"

	"log/slog"

	"github.com/lestrrat-go/jwx/jwk"
	"github.com/ory/fosite"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/util"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
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

	Logger *slog.Logger `json:"-"`
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

func (s *Server) ListenAndServe(ctx context.Context) error {
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

	return util.ServeHandler(ctx, mux, listener)
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
	s.mgmtApiClient = managementv1.NewManagementClient(cc)
	return nil
}

func (s *Server) configureWebServer(_ context.Context, mux *http.ServeMux) {
	mux.Handle("/web/", http.FileServer(http.FS(webFS)))
}

func (in *ServerConfig) DeepCopyInto(out *ServerConfig) {
	util.DeepCopyInto(out, in)
}

func (in *ServerConfig) DeepCopy() *ServerConfig {
	return util.DeepCopy(in)
}

package noauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"

	oautherrors "github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/generates"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ServerConfig struct {
	ClientID              string `mapstructure:"clientID"`
	ClientSecret          string `mapstructure:"clientSecret"`
	ClientDomain          string `mapstructure:"clientDomain"`
	ManagementAPIEndpoint string `mapstructure:"managementAPIEndpoint"`
	Port                  string `mapstructure:"port"`

	Logger *zap.SugaredLogger
}

type Server struct {
	ServerConfig
	mgmtApiClient management.ManagementClient
	key           jwk.Key
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

	return &Server{
		ServerConfig: *conf,
		key:          key,
	}
}

type templateData struct {
	Users []string
	Port  int
}

//go:embed web
var webFS embed.FS

func (s *Server) Run(ctx context.Context) error {
	lg := s.Logger
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", s.Port))
	if err != nil {
		return err
	}

	lg.With(
		"address", listener.Addr(),
	).Info("noauth server starting")
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()

	if err := s.connectToManagementAPI(ctx); err != nil {
		return err
	}

	s.configureOAuthServer(mux)
	s.configureWebServer(mux, port)

	server := http.Server{
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	waitctx.AddOne(ctx)
	go func() {
		defer waitctx.Done(ctx)
		<-ctx.Done()
		lg.Info("noauth server shutting down")
		if err := server.Close(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("an error occurred while shutting down the server")
		}
	}()
	err = server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) connectToManagementAPI(ctx context.Context) error {
	cc, err := grpc.DialContext(ctx, s.ManagementAPIEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	waitctx.AddOne(ctx)
	go func() {
		defer waitctx.Done(ctx)
		<-ctx.Done()
		cc.Close()
	}()
	s.mgmtApiClient = management.NewManagementClient(cc)
	return nil
}

func (s *Server) configureOAuthServer(mux *http.ServeMux) {
	manager := manage.NewDefaultManager()
	tokenStore, err := store.NewMemoryTokenStore()
	manager.MustTokenStorage(tokenStore, err)

	encodedKey, err := jwk.Pem(s.key)
	if err != nil {
		panic(err)
	}
	manager.MapAccessGenerate(generates.NewJWTAccessGenerate("", encodedKey, jwt.SigningMethodRS256))

	clientStore := store.NewClientStore()
	clientStore.Set(s.ClientID, &models.Client{
		ID:     s.ClientID,
		Secret: s.ClientSecret,
		Domain: s.ClientDomain,
	})
	manager.MapClientStorage(clientStore)

	srv := server.NewDefaultServer(manager)
	srv.SetClientInfoHandler(server.ClientFormHandler)
	srv.SetInternalErrorHandler(func(err error) (re *oautherrors.Response) {
		log.Println("Internal Error:", err.Error())
		return
	})
	srv.SetResponseErrorHandler(func(re *oautherrors.Response) {
		log.Println("Response Error:", re.Error.Error())
	})
	srv.SetUserAuthorizationHandler(userAuthHandler)
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		redirect := r.URL
		redirect.Path = "/login"
		http.Redirect(w, r, redirect.String(), http.StatusFound)
	})
	mux.HandleFunc("/callbacks/login", func(w http.ResponseWriter, r *http.Request) {
		err := srv.HandleAuthorizeRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		srv.HandleTokenRequest(w, r)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		ti, err := srv.ValidationBearerToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		sub := ti.GetUserID()
		name := sub
		email := sub
		// world's best email validation
		if !strings.Contains(sub, "@") {
			email += "@example.com"
		} else {
			name = strings.Split(sub, "@")[0]
		}

		claims := map[string]interface{}{
			"sub":           sub,
			"name":          name,
			"email":         email,
			"grafana_roles": []string{"Admin"},
		}
		data, err := json.Marshal(claims)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		issuerUrl := "http://" + r.Host
		response := openid.WellKnownConfiguration{
			Issuer:           issuerUrl,
			AuthEndpoint:     issuerUrl + "/authorize",
			TokenEndpoint:    issuerUrl + "/token",
			UserinfoEndpoint: issuerUrl + "/userinfo",
			JwksUri:          issuerUrl + "/.well-known/jwks.json",
		}
		data, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		set := jwk.NewSet()
		pub, err := s.key.PublicKey()
		if err != nil {
			panic(err)
		}
		set.Add(pub)
		data, err := json.Marshal(set)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
}

func (s *Server) configureWebServer(mux *http.ServeMux, port int) {
	mux.Handle("/web/", http.FileServer(http.FS(webFS)))

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		rbs, err := s.mgmtApiClient.ListRoleBindings(r.Context(), &emptypb.Empty{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		knownusers := map[string]struct{}{}
		for _, rb := range rbs.Items {
			for _, subject := range rb.Subjects {
				knownusers[subject] = struct{}{}
			}
		}
		allUsersSorted := make([]string, 0, len(knownusers))
		for u := range knownusers {
			allUsersSorted = append(allUsersSorted, u)
		}
		sort.Strings(allUsersSorted)
		data := templateData{
			Users: allUsersSorted,
			Port:  port,
		}
		tmpl := template.New("index.html")
		siteTemplate, err := tmpl.ParseFS(webFS, "web/templates/index.html")
		if err != nil {
			return
		}
		siteTemplate.Execute(w, data)
	})
}

func userAuthHandler(w http.ResponseWriter, r *http.Request) (userID string, err error) {
	username := r.FormValue("login_as")
	if username == "" {
		return "", oautherrors.ErrInvalidRequest
	}
	return username, nil
}

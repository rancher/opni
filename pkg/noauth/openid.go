package noauth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/jwk"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/storage"
	openidauth "github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

type NoauthClient interface {
	fosite.Client
	fosite.OpenIDConnectClient
	fosite.ResponseModeClient
}

type defaultNoauthClient struct {
	fosite.Client
	fosite.OpenIDConnectClient
	fosite.ResponseModeClient
}

var (
	allowedGrantTypes = []string{
		"authorization_code",
		"client_credentials",
		"refresh_token",
	}
	allowedResponseModes = []string{
		"query",
		"fragment",
		"form_post",
	}
	allowedResponseTypes = []string{
		"code",
		"token",
		"id_token",
		"id_token token",
		"code id_token",
		"code token",
		"code id_token token",
	}
	allowedScopes = []string{
		"openid",
		"profile",
		"offline_access",
		"name",
		"email",
		"email_verified",
		"created_at",
		"identities",
	}
	allowedClaims = []string{
		"aud",
		"auth_time",
		"created_at",
		"email",
		"email_verified",
		"exp",
		"family_name",
		"given_name",
		"iat",
		"identities",
		"iss",
		"name",
		"nickname",
		"phone_number",
		"picture",
		"sub",
	}
)

func newClient(conf *ServerConfig) NoauthClient {
	hash, err := bcrypt.GenerateFromPassword([]byte(conf.ClientSecret), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	conf.Logger.With(
		"client_id", conf.ClientID,
		"client_secret", conf.ClientSecret,
		"redirect_uri", conf.RedirectURI,
	).Debug("configured noauth client")
	oidcClient := &fosite.DefaultOpenIDConnectClient{
		DefaultClient: &fosite.DefaultClient{
			ID:            conf.ClientID,
			Secret:        hash,
			GrantTypes:    allowedGrantTypes,
			ResponseTypes: allowedResponseTypes,
			RedirectURIs:  []string{conf.RedirectURI},
			Scopes:        allowedScopes,
			Public:        false,
		},
		TokenEndpointAuthMethod: "client_secret_post",
	}
	return &defaultNoauthClient{
		Client:              oidcClient,
		OpenIDConnectClient: oidcClient,
		ResponseModeClient: &fosite.DefaultResponseModeClient{
			DefaultClient: oidcClient.DefaultClient,
			ResponseModes: []fosite.ResponseModeType{
				fosite.ResponseModeDefault,
				fosite.ResponseModeFormPost,
				fosite.ResponseModeQuery,
				fosite.ResponseModeFragment,
			},
		},
	}
}

func newOAuthProvider(conf *ServerConfig, key *rsa.PrivateKey) fosite.OAuth2Provider {
	var secret = make([]byte, 32)
	_, err := rand.Read(secret)
	if err != nil {
		panic(err)
	}
	config := &fosite.Config{
		EnforcePKCE:                false,
		SendDebugMessagesToClients: conf.Debug,
		GlobalSecret:               secret,
	}
	store := storage.NewMemoryStore()
	store.Clients[conf.ClientID] = newClient(conf)
	return compose.ComposeAllEnabled(config, store, key)
}

func newSession(cfg *ServerConfig, user string) *openid.DefaultSession {
	s := openid.NewDefaultSession()
	s.Claims.Issuer = cfg.Issuer
	s.Claims.Subject = user
	return s
}

func (s *Server) configureOAuthServer(mux *http.ServeMux) {
	mux.HandleFunc("/oauth2/authorize", s.handleAuthorizeRequest)
	mux.HandleFunc("/oauth2/token", s.handleTokenRequest)
	mux.HandleFunc("/oauth2/revoke", s.handleRevokeRequest)
	mux.HandleFunc("/oauth2/userinfo", s.handleUserInfoRequest)

	mux.HandleFunc("/oauth2/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		issuerUrl := "http://" + r.Host + "/oauth2"
		response := openidauth.WellKnownConfiguration{
			Issuer:                            issuerUrl,
			AuthEndpoint:                      issuerUrl + "/authorize",
			TokenEndpoint:                     issuerUrl + "/token",
			UserinfoEndpoint:                  issuerUrl + "/userinfo",
			RevocationEndpoint:                issuerUrl + "/revoke",
			JwksUri:                           issuerUrl + "/.well-known/jwks.json",
			ScopesSupported:                   allowedScopes,
			ResponseTypesSupported:            allowedResponseTypes,
			ResponseModesSupported:            allowedResponseModes,
			ClaimsSupported:                   allowedClaims,
			IDTokenSigningAlgValuesSupported:  []string{"RS256"},
			TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
		}
		data, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	mux.HandleFunc("/oauth2/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/jwk-set+json")
		w.Write(data)
	})

}

func (s *Server) handleAuthorizeRequest(w http.ResponseWriter, r *http.Request) {
	lg := s.Logger.With(
		"remote_addr", r.RemoteAddr,
		"request", r.RequestURI,
	)

	lg.Info("handling authorization request")

	ctx := r.Context()
	ar, err := s.noauthProvider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("authorize request failed")
		s.noauthProvider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}

	r.ParseForm()
	username := r.Form.Get("username")
	if username == "" {
		lg.Debug("no username provided, rendering login page")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderLoginPage(w, r)
		return
	}
	lg.With(
		"user", username,
		"scopes", ar.GetRequestedScopes(),
		"id", ar.GetClient().GetID(),
		"gt", ar.GetClient().GetGrantTypes(),
		"rt", ar.GetClient().GetResponseTypes(),
	).Debug("username provided")

	for _, scope := range ar.GetRequestedScopes() {
		ar.GrantScope(scope)
	}

	session := newSession(&s.ServerConfig, username)
	session.Claims.ExpiresAt = time.Now().Add(24 * time.Hour)
	if ar.GetGrantedScopes().Has("email") {
		session.Claims.Add("email", emailify(username))
	}
	if ar.GetGrantedScopes().Has("profile") {
		session.Claims.Add("grafana_role", "Admin")
	}

	resp, err := s.noauthProvider.NewAuthorizeResponse(ctx, ar, session)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("authorize response failed")
		s.noauthProvider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}

	lg.With(
		"code", resp.GetCode(),
	).Debug("sending code")

	s.noauthProvider.WriteAuthorizeResponse(ctx, w, ar, resp)
}

func (s *Server) handleTokenRequest(w http.ResponseWriter, r *http.Request) {
	lg := s.Logger.With(
		"remote_addr", r.RemoteAddr,
		"request", r.RequestURI,
	)

	lg.Debug("handling token request")
	ctx := r.Context()
	session := newSession(&s.ServerConfig, "")
	ar, err := s.noauthProvider.NewAccessRequest(ctx, r, session)

	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("token request failed")
		s.noauthProvider.WriteAccessError(ctx, w, ar, err)
		return
	}
	lg.With(
		"client", ar.GetClient().GetID(),
		"gt", ar.GetClient().GetGrantTypes(),
		"rt", ar.GetClient().GetResponseTypes(),
	).Debug("access request accepted")

	if ar.GetGrantTypes().ExactOne("client_credentials") {
		for _, scope := range ar.GetRequestedScopes() {
			ar.GrantScope(scope)
		}
	}

	response, err := s.noauthProvider.NewAccessResponse(ctx, ar)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("token response failed")
		s.noauthProvider.WriteAccessError(ctx, w, ar, err)
		return
	}

	lg.With(
		"type", response.GetTokenType(),
		"access_token", response.GetAccessToken(),
		"id_token", response.GetExtra("id_token"),
		"expires_in", response.GetExtra("expires_in"),
		"refresh_token", response.GetExtra("refresh_token"),
	).Debug("sending token response")

	s.noauthProvider.WriteAccessResponse(ctx, w, ar, response)
}

func (s *Server) handleRevokeRequest(rw http.ResponseWriter, req *http.Request) {
	err := s.noauthProvider.NewRevocationRequest(req.Context(), req)
	s.noauthProvider.WriteRevocationResponse(req.Context(), rw, err)
}

func (s *Server) handleUserInfoRequest(rw http.ResponseWriter, req *http.Request) {
	lg := s.Logger.With(
		"remote_addr", req.RemoteAddr,
		"request", req.RequestURI,
	)
	lg.Debug("handling user info request")

	session := newSession(&s.ServerConfig, "")
	token := req.Header.Get("Authorization")
	if token == "" {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte("bearer token required"))
		return
	}
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer"))
	tokenType, ar, err := s.noauthProvider.IntrospectToken(req.Context(), token, fosite.AccessToken, session)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("user info request failed")
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte("invalid token"))
		return
	}
	lg.With(
		"token", token,
		"type", tokenType,
	).Debug("token received")

	if tokenType != fosite.AccessToken {
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write([]byte("token is not an access token"))
		return
	}
	if ar == nil {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte("token is inactive"))
		return
	}
	openidSession, ok := ar.GetSession().(*openid.DefaultSession)
	if !ok {
		lg.Error("user info request fetched a non-openid session")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	jsonData, err := json.Marshal(openidSession.Claims.ToMapClaims())
	if err != nil {
		lg.Error("failed to marshal openid claims")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	lg.With(
		"claims", string(jsonData),
	).Debug("sending user info response")
	rw.WriteHeader(http.StatusOK)
	rw.Write(jsonData)
}

func emailify(username string) string {
	if strings.Contains(username, "@") {
		return username
	}
	return username + "@example.com"
}

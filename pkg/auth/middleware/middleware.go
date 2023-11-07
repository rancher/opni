package middleware

import (
	"bytes"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/auth/local"
	"github.com/rancher/opni/pkg/proxy"
	"github.com/rancher/opni/pkg/util/oidc"
	ginoauth2 "github.com/zalando/gin-oauth2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

const (
	authRealm          = "Authorization Required"
	authenticateHeader = "WWW-Authenticate"
)

type MultiMiddleware struct {
	Logger             *slog.Logger
	Config             *oauth2.Config
	IdentifyingClaim   string
	UseOIDC            bool
	LocalAuthenticator local.LocalAuthenticator
}

func (m *MultiMiddleware) setUser(tc *ginoauth2.TokenContainer, ctx *gin.Context) bool {
	userID := oidc.SubjectFromClaims(m.Logger, tc.Token, m.IdentifyingClaim)
	if userID == nil {
		m.Logger.Warn("no user info in jwt")
		return false
	}
	ctx.Set(proxy.SubjectKey, *userID)
	return true
}

func (m *MultiMiddleware) basicAuthPassword(value string) []byte {
	basicPrefix := "Basic "
	if !strings.HasPrefix(value, basicPrefix) {
		return []byte{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, basicPrefix))
	if err != nil {
		m.Logger.With("err", err.Error).Error("failed to decode auth header")
		return []byte{}
	}

	split := bytes.Split(payload, []byte(":"))
	if len(split) != 2 {
		m.Logger.Error("invalid basic auth header")
		return []byte{}
	}
	if !bytes.Equal(bytes.ToLower(split[0]), []byte("admin")) {
		return []byte{}
	}
	return split[1]
}

func (m *MultiMiddleware) Handler(authCheck ...ginoauth2.AccessCheckFunction) gin.HandlerFunc {
	if m.UseOIDC {
		authChain := []ginoauth2.AccessCheckFunction{m.setUser}
		authChain = append(authChain, authCheck...)
		return ginoauth2.AuthChain(m.Config.Endpoint, authChain...)
	}
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			c.Header(authenticateHeader, authRealm)
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		password := m.basicAuthPassword(authHeader)
		if len(password) < 1 {
			c.Header(authenticateHeader, authRealm)
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		err := m.LocalAuthenticator.ComparePassword(c, password)
		if err == nil {
			c.Set(proxy.SubjectKey, "OPNI_admin")
			return
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			c.Header(authenticateHeader, authRealm)
			c.AbortWithStatus(http.StatusUnauthorized)
		}
		m.Logger.With("error", err.Error()).Error("password verification failed")
		c.AbortWithStatus(http.StatusInternalServerError)
	}
}

type AuthTypeResponse struct {
	AuthType auth.AuthType `json:"type"`
}

func (m *MultiMiddleware) GetAuthType(ctx *gin.Context) {
	if m.UseOIDC {
		ctx.Render(http.StatusOK, render.JSON{
			Data: AuthTypeResponse{
				AuthType: auth.AuthTypeOIDC,
			},
		})
		return
	}

	ctx.Render(http.StatusOK, render.JSON{
		Data: AuthTypeResponse{
			AuthType: auth.AuthTypeBasic,
		},
	})
}

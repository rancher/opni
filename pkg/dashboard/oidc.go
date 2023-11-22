package dashboard

import (
	"log/slog"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"golang.org/x/oauth2"
)

// AuthDataSource provides a way to obtain data which the auth
// middleware and proxies need.
type AuthDataSource interface {
	StorageBackend() storage.Backend
}

type oidcHandler struct {
	logger   *slog.Logger
	config   *oauth2.Config
	provider *oidc.Provider
	state    string
}

func (h *oidcHandler) handleRedirect(ctx *gin.Context) {
	h.state = oauth2.GenerateVerifier()

	http.Redirect(ctx.Writer, ctx.Request,
		h.config.AuthCodeURL(h.state),
		http.StatusFound,
	)
}

func (h *oidcHandler) handleCallback(ctx *gin.Context) {
	if ctx.Request.URL.Query().Get("state") != h.state {
		h.logger.Error("invalid state in callback")
		ctx.AbortWithStatus(http.StatusBadRequest)
	}
	oauth2Token, err := h.config.Exchange(ctx, ctx.Request.URL.Query().Get("code"))
	if err != nil {
		h.logger.With(logger.Err(err)).Error("failed to exchange token")
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		h.logger.Error("missing id token")
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}

	verifier := h.provider.Verifier(&oidc.Config{
		ClientID: h.config.ClientID,
	})
	_, err = verifier.Verify(ctx, rawIDToken)
	if err != nil {
		h.logger.With(logger.Err(err)).Error("failed to verify id token")
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}
}

package management

import (
	"log/slog"
	"net/http"

	"github.com/coreos/go-oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

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
		h.logger.With("error", err).Error("failed to exchange token")
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
		h.logger.With("error", err).Error("failed to verify id token")
		ctx.AbortWithStatus(http.StatusInternalServerError)
	}
}

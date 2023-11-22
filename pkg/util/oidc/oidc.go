package oidc

import (
	"fmt"
	"log/slog"

	"github.com/lestrrat-go/jwx/jwt"
	"github.com/lestrrat-go/jwx/jwt/openid"
	"github.com/rancher/opni/pkg/logger"
	"golang.org/x/oauth2"
)

func SubjectFromClaims(lg *slog.Logger, token *oauth2.Token, claimName string) *string {
	rawToken, ok := token.Extra("id_token").(string)
	if !ok {
		lg.Error("id token missing from oauth2 token")
		return nil
	}

	j, err := jwt.ParseString(rawToken,
		jwt.WithToken(openid.New()),
		jwt.WithValidate(true),
	)
	if err != nil {
		lg.With(logger.Err(err)).Error("failed to validate jwt")
	}

	claims := j.PrivateClaims()

	claimValue, ok := claims[claimName]
	if !ok {
		lg.Error(fmt.Sprintf("claim %s not present in payload", claimName))
	}

	claimString, ok := claimValue.(string)
	if !ok {
		lg.Error(fmt.Sprintf("claim %s not string in payload", claimName))
	}
	return &claimString
}

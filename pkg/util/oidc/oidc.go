package oidc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/oauth2"
)

func SubjectFromClaims(lg *slog.Logger, token *oauth2.Token, claimName string) *string {
	rawToken, ok := token.Extra("id_token").(string)
	if !ok {
		lg.Error("id token missing from oauth2 token")
		return nil
	}
	jwtParts := strings.Split(rawToken, ".")
	if len(jwtParts) < 2 {
		lg.Error(fmt.Sprintf("malformed jwt, only got %d parts", len(jwtParts)))
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(jwtParts[1])
	if err != nil {
		lg.With(
			"error", err.Error(),
		).Error("failed to decode jwt claims")
	}
	claims := map[string]interface{}{}
	err = json.Unmarshal(payload, &claims)
	if err != nil {
		lg.With(
			"error", err.Error(),
		).Error("failed to unmarshal claims")
	}

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

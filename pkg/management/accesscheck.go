package management

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/storage"
	ginoauth2 "github.com/zalando/gin-oauth2"
	"golang.org/x/oauth2"
)

type accessChecker struct {
	client apiextensions.ManagementAPIExtensionClient
	logger *slog.Logger
	store  storage.RoleBindingStore
}

func (c *accessChecker) CheckAccessForExtension(tc *ginoauth2.TokenContainer, ctx *gin.Context) bool {
	// TODO set this from oauth config
	subjectField := "user"
	userID := c.userFromToken(*tc.Token, subjectField)
	if userID == nil {
		c.logger.Warn("no user info in jwt")
		return false
	}
	roleList, err := c.fetchRoles(ctx, *userID)
	if err != nil {
		c.logger.With(
			"error", err.Error(),
		).Error("failed to fetch role list")
		return false
	}
	check, err := c.client.Authorized(ctx, &apiextensions.AuthzRequest{
		RoleList: roleList,
		Details: &apiextensions.RequestDetails{
			Path: ctx.FullPath(),
			Verb: ctx.Request.Method,
		},
	})
	if err != nil {
		c.logger.With(
			"error", err.Error(),
		).Error("failed to check authorization")
		return false
	}
	return check.GetAuthorized()
}

func (c *accessChecker) userFromToken(token oauth2.Token, claimName string) *string {
	rawToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.logger.Error("id token missing from oauth2 token")
		return nil
	}
	jwtParts := strings.Split(rawToken, ".")
	if len(jwtParts) < 2 {
		c.logger.Error(fmt.Sprintf("malformed jwt, only got %d parts", len(jwtParts)))
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(jwtParts[1])
	if err != nil {
		c.logger.With(
			"error", err.Error(),
		).Error("failed to decode jwt claims")
	}
	claims := map[string]interface{}{}
	err = json.Unmarshal(payload, &claims)
	if err != nil {
		c.logger.With(
			"error", err.Error(),
		).Error("failed to unmarshal claims")
	}

	claimValue, ok := claims[claimName]
	if !ok {
		c.logger.Error(fmt.Sprintf("claim %s not present in payload", claimName))
	}

	claimString, ok := claimValue.(string)
	if !ok {
		c.logger.Error(fmt.Sprintf("claim %s not string in payload", claimName))
	}
	return &claimString
}

func (c *accessChecker) fetchRoles(ctx context.Context, userID string) (*corev1.ReferenceList, error) {
	bindings, err := c.store.ListRoleBindings(ctx)
	if err != nil {
		return nil, err
	}
	roleList := &corev1.ReferenceList{}
	for _, binding := range bindings.GetItems() {
		if slices.Contains(binding.GetSubjects(), userID) {
			roleList.Items = append(roleList.Items, &corev1.Reference{
				Id: binding.RoleId,
			})
		}
	}
	return roleList, nil
}

package management

import (
	"context"
	"log/slog"
	"slices"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/oidc"
	ginoauth2 "github.com/zalando/gin-oauth2"
)

type accessChecker struct {
	client apiextensions.ManagementAPIExtensionClient
	logger *slog.Logger
	store  storage.RoleBindingStore
}

func (c *accessChecker) CheckAccessForExtension(tc *ginoauth2.TokenContainer, ctx *gin.Context) bool {
	// TODO set this from oauth config
	subjectField := "user"
	userID := oidc.SubjectFromClaims(c.logger, tc.Token, subjectField)
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

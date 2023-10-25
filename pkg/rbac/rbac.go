package rbac

import (
	"context"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type RBACHeader map[string]*corev1.ReferenceList

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	AccessHeader(context.Context, *corev1.ReferenceList) (RBACHeader, error)
}

func AuthorizedUserID(c *gin.Context) (string, bool) {
	userId, exists := c.Get(UserIDKey)
	if !exists {
		return "", false
	}
	return userId.(string), true
}

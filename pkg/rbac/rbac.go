package rbac

import (
	"context"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	SubjectAccess(context.Context, *corev1.SubjectAccessRequest) (*corev1.ReferenceList, error)
}

func AuthorizedUserID(c *gin.Context) (string, bool) {
	userId, exists := c.Get(UserIDKey)
	if !exists {
		return "", false
	}
	return userId.(string), true
}

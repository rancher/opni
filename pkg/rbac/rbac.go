package rbac

import (
	"context"

	"github.com/gofiber/fiber/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	SubjectAccess(context.Context, *corev1.SubjectAccessRequest) (*corev1.ReferenceList, error)
}

func AuthorizedUserID(c *fiber.Ctx) (string, bool) {
	userId := c.Locals(UserIDKey)
	if userId == nil {
		return "", false
	}
	return userId.(string), true
}

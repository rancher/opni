package rbac

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/core"
)

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	SubjectAccess(context.Context, *core.SubjectAccessRequest) (*core.ReferenceList, error)
}

func AuthorizedUserID(c *fiber.Ctx) (string, bool) {
	userId := c.Locals(UserIDKey)
	if userId == nil {
		return "", false
	}
	return userId.(string), true
}

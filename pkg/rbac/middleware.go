package rbac

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/core"
)

type middleware struct {
	provider Provider
}

func (m *middleware) Handle(c *fiber.Ctx) error {
	userID := c.Locals(UserIDKey)
	if userID == nil {
		return c.Next()
	}
	clusters, err := m.provider.SubjectAccess(context.Background(), &core.SubjectAccessRequest{
		Subject: userID.(string),
	})
	if err != nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if len(clusters.Items) == 0 {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	ids := make([]string, len(clusters.Items))
	for i, cluster := range clusters.Items {
		ids[i] = cluster.Id
	}
	c.Request().Header.Set("X-Scope-OrgID", strings.Join(ids, "|"))
	return c.Next()
}

func NewMiddleware(provider Provider) func(*fiber.Ctx) error {
	mw := &middleware{
		provider: provider,
	}
	return mw.Handle
}

package rbac

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type middleware struct {
	provider Provider
}

func (m *middleware) Handle(c *fiber.Ctx) error {
	userID := c.Locals(UserIDKey)
	if userID == nil {
		return c.Next()
	}
	tenants, err := m.provider.ListTenantsForUser(context.Background(), userID.(string))
	if err != nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if len(tenants) == 0 {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Request().Header.Set("X-Scope-OrgID", strings.Join(tenants, "|"))
	return c.Next()
}

func NewMiddleware(provider Provider) func(*fiber.Ctx) error {
	mw := &middleware{
		provider: provider,
	}
	return mw.Handle
}

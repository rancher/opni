package test

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-monitoring/pkg/rbac"
)

type AuthStrategy string

const (
	AuthStrategyDenyAll            AuthStrategy = "deny-all"
	AuthStrategyUserIDInAuthHeader AuthStrategy = "user-id-in-auth-header"
)

type TestAuthMiddleware struct {
	Strategy AuthStrategy
}

func (m *TestAuthMiddleware) Description() string {
	return "Test"
}

func (m *TestAuthMiddleware) Handle(c *fiber.Ctx) error {
	switch m.Strategy {
	case AuthStrategyDenyAll:
		return c.SendStatus(fiber.StatusUnauthorized)
	case AuthStrategyUserIDInAuthHeader:
		userId := c.Get("Authorization")
		if userId == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		c.Request().Header.Del("Authorization")
		c.Locals(rbac.UserIDKey, userId)
		return c.Next()
	default:
		panic("unknown auth strategy")
	}
}

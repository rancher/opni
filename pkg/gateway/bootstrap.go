package gateway

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
)

func (g *Gateway) setupBootstrapEndpoint(app *fiber.App) {
	app.Post("/bootstrap", bootstrap.ServerConfig{
		RootCA:     g.rootCA,
		Keypair:    g.keypair,
		TokenStore: bootstrap.NewVolatileSecretStore(),
	}.Handle).Use(limiter.New())
}

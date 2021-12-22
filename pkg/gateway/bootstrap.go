package gateway

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/storage"
)

func (g *Gateway) setupBootstrapEndpoint(app *fiber.App) {
	app.Post("/bootstrap", bootstrap.ServerConfig{
		RootCA:     g.rootCA,
		Keypair:    g.keypair,
		TokenStore: storage.NewVolatileSecretStore(),
	}.Handle).Use(limiter.New())
}

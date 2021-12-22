package gateway

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/storage"
)

func (g *Gateway) setupBootstrapEndpoint(app *fiber.App) {
	// test code
	test := storage.NewVolatileTokenStore()
	token, err := test.CreateToken(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(token.EncodeHex())

	app.Post("/bootstrap", bootstrap.ServerConfig{
		RootCA:     g.rootCA,
		Keypair:    g.keypair,
		TokenStore: test,
	}.Handle).Use(limiter.New())
}

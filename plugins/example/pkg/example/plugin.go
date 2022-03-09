package example

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ExamplePlugin struct {
	UnimplementedExampleAPIExtensionServer
	Logger hclog.Logger
}

func (s *ExamplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (s *ExamplePlugin) UseManagementAPI(api management.ManagementClient) {
	lg := s.Logger
	lg.Info("querying management API...")
	var list *management.APIExtensionInfoList
	for {
		var err error
		list, err = api.APIExtensions(context.Background(), &emptypb.Empty{})
		if err != nil {
			log.Fatal(err)
		}
		if len(list.Items) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	for _, ext := range list.Items {
		lg.Info("found API extension service", "name", ext.ServiceDesc.GetName())
	}
}

func (s *ExamplePlugin) UseKeyValueStore(kv system.KVStoreClient) {
	lg := s.Logger
	err := kv.Put("foo", &EchoRequest{
		Message: "hello",
	})
	if err != nil {
		lg.Error("kv store error", "error", err)
	}

	out := &EchoRequest{}
	err = kv.Get("foo", out)
	if err != nil {
		lg.Error("kv store error", "error", err)
	}
	lg.Info("successfully retrieved stored value", "message", out.Message)
}

func (s *ExamplePlugin) ConfigureRoutes(app *fiber.App) {
	app.Get("/example", func(c *fiber.Ctx) error {
		s.Logger.Debug("handling /example")
		return c.JSON(map[string]string{
			"message": "hello world",
		})
	})
}

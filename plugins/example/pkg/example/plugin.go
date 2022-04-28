package example

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	gatewayext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ExamplePlugin struct {
	UnsafeExampleAPIExtensionServer
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

func (p *ExamplePlugin) CanInstall() error {
	return nil
}

func (p *ExamplePlugin) Install(cluster *core.Reference) error {
	return nil
}

func (p *ExamplePlugin) InstallerTemplate() string {
	return `foo {{ arg "input" "Input" "+omitEmpty" "+default:default" "+format:--bar={{ value }}" }} ` +
		`{{ arg "toggle" "Toggle" "+omitEmpty" "+default:false" "+format:--reticulateSplines" }} ` +
		`{{ arg "select" "Select" "" "foo" "bar" "baz" "+omitEmpty" "+default:foo" "+format:--select={{ value }}" }}`
}

func Scheme() meta.Scheme {
	scheme := meta.NewScheme()
	p := &ExamplePlugin{
		Logger: logger.NewForPlugin(),
	}
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(&ExampleAPIExtension_ServiceDesc, p))
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID,
		gatewayext.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin("test", p))
	return scheme
}

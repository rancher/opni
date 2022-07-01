package example

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-hclog"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	gatewayext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	unaryext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/unary"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ExamplePlugin struct {
	UnsafeExampleAPIExtensionServer
	UnsafeExampleUnaryExtensionServer
	system.UnimplementedSystemPluginClient
	Logger hclog.Logger
}

func (s *ExamplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (s *ExamplePlugin) Hello(context.Context, *emptypb.Empty) (*EchoResponse, error) {
	return &EchoResponse{
		Message: "Hello World",
	}, nil
}

func (s *ExamplePlugin) UseManagementAPI(api managementv1.ManagementClient) {
	lg := s.Logger
	lg.Info("querying management API...")
	var list *managementv1.APIExtensionInfoList
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

func (s *ExamplePlugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	kv := system.NewKVStoreClient[*EchoRequest](context.Background(), client)
	lg := s.Logger
	err := kv.Put("foo", &EchoRequest{
		Message: "hello",
	})
	if err != nil {
		lg.Error("kv store error", "error", err)
	}

	value, err := kv.Get("foo")
	if err != nil {
		lg.Error("kv store error", "error", err)
	}
	lg.Info("successfully retrieved stored value", "message", value.Message)
}

func (s *ExamplePlugin) ConfigureRoutes(app *gin.Engine) {
	app.GET("/example", func(c *gin.Context) {
		s.Logger.Debug("handling /example")
		c.JSON(http.StatusOK, map[string]string{
			"message": "hello world",
		})
	})
}

func (p *ExamplePlugin) CanInstall() error {
	return nil
}

func (p *ExamplePlugin) Install(cluster *corev1.Reference) error {
	return nil
}

func (p *ExamplePlugin) Uninstall(clustre *corev1.Reference) error {
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
	scheme.Add(unaryext.UnaryAPIExtensionPluginID, unaryext.NewPlugin(&ExampleUnaryExtension_ServiceDesc, p))
	return scheme
}
